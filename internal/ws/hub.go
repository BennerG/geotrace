package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/BennerG/geotrace/internal/store"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
	sendBuffer     = 32
)

type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts events to them.
type Hub struct {
	clients        map[*client]struct{}
	broadcast      <-chan *store.Event
	register       chan *client
	unregister     chan *client
	allowedOrigins map[string]struct{}
}

func NewHub(broadcast <-chan *store.Event, allowedOrigins []string) *Hub {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		origins[o] = struct{}{}
	}
	return &Hub{
		clients:        make(map[*client]struct{}),
		broadcast:      broadcast,
		register:       make(chan *client),
		unregister:     make(chan *client),
		allowedOrigins: origins,
	}
}

// Run is the hub's main loop. It must run in exactly one goroutine —
// all map access is intentionally single-threaded here.
func (h *Hub) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case c := <-h.register:
			h.clients[c] = struct{}{}
			slog.Debug("ws: client connected", "total", len(h.clients))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				slog.Debug("ws: client disconnected", "total", len(h.clients))
			}

		case ev := <-h.broadcast:
			msg := eventToJSON(ev)
			if msg == nil {
				continue
			}
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// client's send buffer is full — drop and disconnect
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}

// ServeHTTP upgrades the HTTP connection to WebSocket and registers the client.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser client
			}
			_, ok := h.allowedOrigins[origin]
			return ok
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "err", err)
		return
	}

	c := &client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, sendBuffer),
	}

	h.register <- c

	go c.writePump()
	c.readPump() // blocks until client disconnects
}

// readPump keeps the connection alive by reading control messages (pings).
// It unregisters the client when the connection closes.
func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// We don't expect messages from the client — this just
		// keeps the connection alive and detects disconnects
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// writePump drains the client's send channel to the WebSocket connection.
// A ticker sends pings to keep the connection alive.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func eventToJSON(ev *store.Event) []byte {
	f := ev.ToGeoJSON()
	if f == nil {
		return nil
	}
	b, err := json.Marshal(f)
	if err != nil {
		slog.Error("ws: marshal event", "err", err)
		return nil
	}
	return b
}

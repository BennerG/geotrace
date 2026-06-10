package enricher

import (
	"context"
	"log/slog"
	"net"

	"github.com/oschwald/geoip2-golang"
	"golang.org/x/sync/errgroup"

	"github.com/BennerG/geotrace/internal/store"
)

// enricher coordinates worker pool
type Enricher struct {
	db        *geoip2.Reader
	st        *store.Store
	events    <-chan *store.Event // raw events from ingest handler
	broadcast chan<- *store.Event // enriched events to WebSocket hub
	workers   int
}

func New(
	mmdbPath string,
	st *store.Store,
	events <-chan *store.Event,
	broadcast chan<- *store.Event,
	workers int,
) (*Enricher, error) {
	db, err := geoip2.Open(mmdbPath)
	if err != nil {
		return nil, err
	}

	return &Enricher{
		db:        db,
		st:        st,
		events:    events,
		broadcast: broadcast,
		workers:   workers,
	}, nil
}

func (e *Enricher) Close() {
	_ = e.db.Close()
}

// run starts the worker pool and uses errgroup to stop workers
func (e *Enricher) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	for i := range e.workers {
		workerID := i + 1
		g.Go(func() error {
			return e.runWorker(ctx, workerID)
		})
	}
	// blocks until fatal
	return g.Wait()
}

// runWorker drains the events channel until ctx is cancelled
func (e *Enricher) runWorker(ctx context.Context, id int) error {
	slog.Debug("enricher worker started", "id", id)

	for {
		select {
		case <-ctx.Done():
			slog.Debug("enricher worker stopping", "id", id)
			return nil

		case ev, ok := <-e.events:
			if !ok {
				// Channel closed — clean shutdown
				return nil
			}
			e.process(ctx, ev)
		}
	}
}

// process geo-enriches a single event and writes it to Postgres
func (e *Enricher) process(ctx context.Context, ev *store.Event) {
	// store ip as IPv4
	if v4 := ev.IP.To4(); v4 != nil {
		ev.IP = v4
	}

	// skip geo lookup for private/loopback IPs
	if !ev.IsPrivate() {
		e.geoLookup(ev)
	}

	if err := e.st.Insert(ctx, ev); err != nil {
		slog.Error("enricher: postgres insert failed",
			"ip", ev.IP.String(),
			"path", ev.Path,
			"err", err,
		)
		return
	}

	// non-blocking send to broadcast channel
	select {
	case e.broadcast <- ev:
	default:
		slog.Debug("enricher: broadcast channel full, dropping live event",
			"id", ev.ID,
		)
	}
}

// geoLookup performs the MaxMind City DB lookup and populates the geo
// fields on the event in-place.
func (e *Enricher) geoLookup(ev *store.Event) {
	ip := ev.IP.To4()
	if ip == nil {
		ip = ev.IP.To16()
	}
	if ip == nil {
		slog.Warn("enricher: could not normalize IP", "ip", ev.IP)
		return
	}

	record, err := e.db.City(net.IP(ip))
	if err != nil {
		slog.Warn("enricher: maxmind lookup failed",
			"ip", ev.IP.String(),
			"err", err,
		)
		return
	}

	// coordinates
	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		lat := record.Location.Latitude
		lon := record.Location.Longitude
		ev.Lat = &lat
		ev.Lon = &lon
	}

	// city name
	if name, ok := record.City.Names["en"]; ok && name != "" {
		ev.City = &name
	}

	// subdivision
	if len(record.Subdivisions) > 0 {
		if name, ok := record.Subdivisions[0].Names["en"]; ok && name != "" {
			ev.Region = &name
		}
	}

	// country
	if name, ok := record.Country.Names["en"]; ok && name != "" {
		ev.Country = &name
	}
	if record.Country.IsoCode != "" {
		code := record.Country.IsoCode
		ev.CountryCode = &code
	}
}

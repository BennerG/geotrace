import { useEffect, useRef, useCallback } from 'react'
import { GeoJSONFeature } from '../types'

type Handler = (feature: GeoJSONFeature) => void

export function useWebSocket(onFeature: Handler) {
  const wsRef = useRef<WebSocket | null>(null)
  const handlerRef = useRef<Handler>(onFeature)

  useEffect(() => {
    handlerRef.current = onFeature
  }, [onFeature])

  const connect = useCallback(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws`)

    ws.onmessage = (e) => {
      try {
        const feature = JSON.parse(e.data) as GeoJSONFeature
        handlerRef.current(feature)
      } catch {
        // malformed message — ignore
      }
    }

    ws.onclose = () => {
      // reconnect after 2s on unexpected close
      setTimeout(connect, 2000)
    }

    wsRef.current = ws
  }, [])

  useEffect(() => {
    connect()
    return () => {
      wsRef.current?.close()
    }
  }, [connect])
}

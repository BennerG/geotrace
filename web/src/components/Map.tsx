import { useEffect, useRef, useCallback } from 'react'
import mapboxgl from 'mapbox-gl'
import { GeoJSONFeature, FeatureCollection, EventProps } from '../types'
import { useWebSocket } from '../hooks/useWebSocket'

mapboxgl.accessToken = import.meta.env.VITE_MAPBOX_TOKEN

const LIVE_SOURCE = 'live-events'
const HIST_SOURCE = 'hist-events'

const emptyCollection = (): FeatureCollection => ({
  type: 'FeatureCollection',
  features: [],
})

interface Props {
  historicalData: FeatureCollection | null
}

export function Map({ historicalData }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<mapboxgl.Map | null>(null)
  const historicalRef = useRef<FeatureCollection | null>(null)
  const liveFeatures = useRef<GeoJSONFeature[]>([])
  const pulseMarkers = useRef<mapboxgl.Marker[]>([])
  const styleReadyRef = useRef(false)

  // initialize map once
  useEffect(() => {
    if (!containerRef.current) return

    const map = new mapboxgl.Map({
      container: containerRef.current,
      style: 'mapbox://styles/mapbox/dark-v11',
      center: [-90, 30],
      zoom: 1.8,
      projection: 'globe',
      attributionControl: false,
    })

    map.addControl(new mapboxgl.NavigationControl(), 'bottom-right')
    map.addControl(new mapboxgl.AttributionControl({ compact: true }), 'bottom-left')
    map.addControl(new CentroidControl(() => {
      const all = [...liveFeatures.current, ...(historicalRef.current?.features ?? [])]
      const center = computeCentroid(all)
      if (!center) return
      map.flyTo({ center, zoom: 4, duration: 1200 })
    }), 'bottom-right')

    map.on('style.load', () => {
      styleReadyRef.current = true
      // atmosphere / fog on globe view
      map.setFog({
        color: 'rgb(10, 14, 26)',
        'high-color': 'rgb(20, 30, 60)',
        'horizon-blend': 0.04,
      })

      // ── Live events source + layers ───────────────────────────────────
      map.addSource(LIVE_SOURCE, {
        type: 'geojson',
        data: emptyCollection(),
        cluster: true,
        clusterMaxZoom: 10,
        clusterRadius: 40,
      })

      // cluster circles
      map.addLayer({
        id: 'live-clusters',
        type: 'circle',
        source: LIVE_SOURCE,
        filter: ['has', 'point_count'],
        paint: {
          'circle-color': [
            'step', ['get', 'point_count'],
            '#1a6b4a', 10,
            '#1e8c5e', 30,
            '#22aa72',
          ],
          'circle-radius': [
            'step', ['get', 'point_count'],
            16, 10, 22, 30, 30,
          ],
          'circle-opacity': 0.85,
          'circle-stroke-width': 1,
          'circle-stroke-color': '#22aa72',
        },
      })

      // cluster count labels
      map.addLayer({
        id: 'live-cluster-count',
        type: 'symbol',
        source: LIVE_SOURCE,
        filter: ['has', 'point_count'],
        layout: {
          'text-field': ['get', 'point_count_abbreviated'],
          'text-font': ['DIN Offc Pro Medium', 'Arial Unicode MS Bold'],
          'text-size': 11,
        },
        paint: { 'text-color': '#e8f5ee' },
      })

      // individual pins
      map.addLayer({
        id: 'live-points',
        type: 'circle',
        source: LIVE_SOURCE,
        filter: ['!', ['has', 'point_count']],
        paint: {
          'circle-color': '#22aa72',
          'circle-radius': 5,
          'circle-opacity': 0.9,
          'circle-stroke-width': 1.5,
          'circle-stroke-color': '#e8f5ee',
        },
      })

      // ── Historical events source + layers ─────────────────────────────
      map.addSource(HIST_SOURCE, {
        type: 'geojson',
        data: emptyCollection(),
        cluster: true,
        clusterMaxZoom: 10,
        clusterRadius: 40,
      })

      if (historicalRef.current) {
        const src = map.getSource(HIST_SOURCE) as mapboxgl.GeoJSONSource
        src.setData(historicalRef.current)
      }

      map.addLayer({
        id: 'hist-clusters',
        type: 'circle',
        source: HIST_SOURCE,
        filter: ['has', 'point_count'],
        paint: {
          'circle-color': [
            'step', ['get', 'point_count'],
            '#1a3a6b', 10,
            '#1e508c', 30,
            '#2266aa',
          ],
          'circle-radius': [
            'step', ['get', 'point_count'],
            16, 10, 22, 30, 30,
          ],
          'circle-opacity': 0.75,
          'circle-stroke-width': 1,
          'circle-stroke-color': '#2266aa',
        },
      })

      map.addLayer({
        id: 'hist-cluster-count',
        type: 'symbol',
        source: HIST_SOURCE,
        filter: ['has', 'point_count'],
        layout: {
          'text-field': ['get', 'point_count_abbreviated'],
          'text-font': ['DIN Offc Pro Medium', 'Arial Unicode MS Bold'],
          'text-size': 11,
        },
        paint: { 'text-color': '#c8ddf5' },
      })

      map.addLayer({
        id: 'hist-points',
        type: 'circle',
        source: HIST_SOURCE,
        filter: ['!', ['has', 'point_count']],
        paint: {
          'circle-color': '#2266aa',
          'circle-radius': 4,
          'circle-opacity': 0.7,
          'circle-stroke-width': 1,
          'circle-stroke-color': '#c8ddf5',
        },
      })

      // —— Click cluster —————————————————————————————————————————————————
      const addClusterClickHandler = (source: string, layerId: string) => {
        map.on('click', layerId, (e) => {
          if (!e.features?.length) return
          const feature = e.features[0]
          const clusterId = feature.properties?.cluster_id
          const src = map.getSource(source) as mapboxgl.GeoJSONSource
          src.getClusterExpansionZoom(clusterId, (err, zoom) => {
            if (err) return
            const coords = (feature.geometry as GeoJSON.Point).coordinates as [number, number]
            map.easeTo({ center: coords, zoom: Number(zoom) + 0.5 })
          })
        })
        map.on('mouseenter', layerId, () => { map.getCanvas().style.cursor = 'pointer' })
        map.on('mouseleave', layerId, () => { map.getCanvas().style.cursor = '' })
      }

      addClusterClickHandler(LIVE_SOURCE, 'live-clusters')
      addClusterClickHandler(HIST_SOURCE, 'hist-clusters')

      // ── Click popup ───────────────────────────────────────────────────
      const popup = new mapboxgl.Popup({
        closeButton: true,
        closeOnClick: true,
        className: 'geotrace-popup',
      })

      const showPopup = async (e: mapboxgl.MapMouseEvent & { features?: mapboxgl.GeoJSONFeature[]}) => {
        if (!e.features?.length) return
        const f = e.features[0]
        const props = f.properties as EventProps
        const coords = (f.geometry as GeoJSON.Point).coordinates.slice() as [number, number]
        const date = new Date(props.created_at).toLocaleString()

        popup.setLngLat(coords).setHTML(`
          <div class="popup-inner">
            <div class="popup-flag">${countryFlag(props.country_code)}</div>
            <div class="popup-city">${props.city || props.country || 'Unknown'}</div>
            <div class="popup-meta">${props.method} ${props.path}</div>
            <div class="popup-meta">${date}</div>
            <div class="popup-ip">${props.ip}</div>
            <div class="popup-summary-loading">loading request summary…</div>
          </div>
        `).addTo(map)

        try {
          const res = await fetch(`/summary?ip=${encodeURIComponent(props.ip)}`)
          if (!res.ok || !popup.isOpen()) return
          const paths = await res.json() as Array<{ method: string; path: string; count: number }>

          const rows = paths.slice(0, 5).map(p =>
            `<div class="popup-path-row">
              <span class="popup-method">${p.method}</span>
              <span class="popup-path-text">${p.path}</span>
              <span class="popup-count">${p.count}×</span>
            </div>`
          ).join('')

          popup.setHTML(`
            <div class="popup-inner">
              <div class="popup-flag">${countryFlag(props.country_code)}</div>
              <div class="popup-city">${props.city || props.country || 'Unknown'}</div>
              <div class="popup-meta">${date}</div>
              <div class="popup-ip">${props.ip}</div>
              <div class="popup-divider"></div>
              <div class="popup-section-label">top paths</div>
              ${rows}
            </div>
          `)
        } catch {
          // summary fetch failed - do nothing, basic info already visible
        }
      }

      map.on('click', 'live-points', showPopup)
      map.on('click', 'hist-points', showPopup)

      map.on('mouseenter', 'live-points', () => { map.getCanvas().style.cursor = 'pointer' })
      map.on('mouseleave', 'live-points', () => { map.getCanvas().style.cursor = '' })
      map.on('mouseenter', 'hist-points', () => { map.getCanvas().style.cursor = 'pointer' })
      map.on('mouseleave', 'hist-points', () => { map.getCanvas().style.cursor = '' })
    })

    mapRef.current = map
    return () => map.remove()
  }, [])

  // update historical layer when data changes
  useEffect(() => {
    historicalRef.current = historicalData
    if (!historicalData) return 

    const map = mapRef.current
    if (!map) return

    const apply = () => {
      const src = map.getSource(HIST_SOURCE) as mapboxgl.GeoJSONSource | undefined
      src?.setData(historicalData)
    }

    if (styleReadyRef.current) {
      apply()
    } else {
      map.once('style.load', apply)
    }
  }, [historicalData])

  // handle incoming live event
  const handleFeature = useCallback((feature: GeoJSONFeature) => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    liveFeatures.current = [...liveFeatures.current.slice(-499), feature]

    const src = map.getSource(LIVE_SOURCE) as mapboxgl.GeoJSONSource | undefined
    src?.setData({
      type: 'FeatureCollection',
      features: liveFeatures.current,
    })

    // pulse marker at the new pin location
    const [lon, lat] = feature.geometry.coordinates
    const el = document.createElement('div')
    el.className = 'pulse-ring'

    const marker = new mapboxgl.Marker({ element: el, anchor: 'center' })
      .setLngLat([lon, lat])
      .addTo(map)

    pulseMarkers.current.push(marker)
    setTimeout(() => {
      marker.remove()
      pulseMarkers.current = pulseMarkers.current.filter(m => m !== marker)
    }, 1200)
  }, [])

  useWebSocket(handleFeature)

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />
}

function countryFlag(code: string): string {
  if (!code || code.length !== 2) return '🌐'
  return String.fromCodePoint(
    ...code.toUpperCase().split('').map(c => 0x1F1E6 + c.charCodeAt(0) - 65)
  )
}

function computeCentroid(features: GeoJSONFeature[]): [number, number] | null {
  const valid = features.filter(f => f.geometry?.coordinates?.length === 2)
  if (!valid.length) return null
  const sumLon = valid.reduce((s, f) => s + f.geometry.coordinates[0], 0)
  const sumLat = valid.reduce((s, f) => s + f.geometry.coordinates[1], 0)
  return [sumLon / valid.length, sumLat / valid.length]
}

class CentroidControl implements mapboxgl.IControl {
  private container!: HTMLElement
  private onClick: () => void

  constructor(onClick: () => void) {
    this.onClick = onClick
  }

  onAdd() {
    this.container = document.createElement('div')
    this.container.className = 'mapboxgl-ctrl mapboxgl-ctrl-group'
    const btn = document.createElement('button')
    btn.title = 'Zoom to traffic centroid'
    btn.style.cssText = 'font-size:16px; cursor:pointer;'
    btn.innerHTML = '⊕'
    btn.onclick = this.onClick
    this.container.appendChild(btn)
    return this.container
  }

  onRemove() {
    this.container.remove()
  }
}

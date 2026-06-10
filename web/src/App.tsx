import { useState, useEffect, useCallback } from 'react'
import { createGlobalStyle } from 'styled-components'
import { Map } from './components/Map'
import { FeatureCollection } from './types'
import { TimeScrubber } from './components/TimeScrubber'
import { StatsBar } from './components/StatsBar'
import { useStats } from './hooks/useStats'

const GlobalStyle = createGlobalStyle`
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

  :root {
    --bg: #080c14;
    --surface: #0d1520;
    --border: #1a2540;
    --green: #22aa72;
    --green-dim: #1a6b4a;
    --blue: #2266aa;
    --text: #e8edf5;
    --text-dim: #6b7a99;
    --font-mono: 'JetBrains Mono', 'Fira Code', monospace;
    --font-sans: 'Inter', system-ui, sans-serif;
  }

  html, body, #root {
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--bg);
    color: var(--text);
    font-family: var(--font-sans);
  }

  /* Mapbox popup */
  .geotrace-popup .mapboxgl-popup-content {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0;
    box-shadow: 0 8px 32px rgba(0,0,0,0.6);
    min-width: 180px;
  }

  .geotrace-popup .mapboxgl-popup-tip {
    border-top-color: var(--border);
  }

  .popup-inner {
    padding: 10px 14px;
  }

  .popup-flag {
    font-size: 22px;
    margin-bottom: 4px;
  }

  .popup-city {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    margin-bottom: 6px;
  }

  .popup-meta {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-dim);
    margin-bottom: 2px;
  }

  .popup-ip {
    font-size: 10px;
    font-family: var(--font-mono);
    color: var(--green);
    margin-top: 6px;
    opacity: 0.7;
  }
  
  .pulse-ring {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    pointer-events: none;
    animation: pulse-out 1.8s ease-out forwards;
  }

  @keyframes pulse-out {
    0% {
      box-shadow: 0 0 0 0px rgba(34, 170, 114, 0.8);
      opacity: 1;
    }
    100% {
      box-shadow: 0 0 0 20px rgba(34, 170, 114, 0);
      opacity: 0;
    }
  }
`

const now = () => new Date().toISOString()
const hourAgo = () => new Date(Date.now() - 3600_000).toISOString()

export default function App() {
  const [historicalData, setHistoricalData] = useState<FeatureCollection | null>(null)
  const [loading, setLoading] = useState(false)
  const [activeWindow, setActiveWindow] = useState({
    from: hourAgo(),
    to: now(),
  })
  const { stats, error: statsError } = useStats(activeWindow.from, activeWindow.to)

  const fetchHistorical = useCallback(async (from: string, to: string) => {
    setActiveWindow({ from, to })
    setLoading(true)
    try {
      const res = await fetch(`/events?from=${from}&to=${to}`)
      if (!res.ok) return
      const data = await res.json() as FeatureCollection
      setHistoricalData(data)
    } catch {
      // network error — silently ignore, map shows live data
    } finally {
      setLoading(false)
    }
  }, [])

  // load last hour on mount
  useEffect(() => {
    fetchHistorical(hourAgo(), now())
  }, [fetchHistorical])

  return (
    <>
      <GlobalStyle />
      <div style={{ width: '100vw', height: '100vh', position: 'relative' }}>
        <Header />
        <Map historicalData={historicalData} />
        <TimeScrubber onFetch={fetchHistorical} loading={loading} />
        <StatsBar stats={stats} error={statsError} />
      </div>
    </>
  )
}

function Header() {
  return (
    <div style={{
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      zIndex: 10,
      padding: '14px 20px',
      display: 'flex',
      alignItems: 'center',
      gap: '12px',
      background: 'linear-gradient(to bottom, rgba(8,12,20,0.95) 0%, rgba(8,12,20,0) 100%)',
      pointerEvents: 'none',
    }}>
      <span style={{
        fontFamily: 'var(--font-mono)',
        fontSize: '13px',
        fontWeight: 600,
        color: 'var(--green)',
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
      }}>
        GeoTrace
      </span>
      <span style={{
        fontFamily: 'var(--font-mono)',
        fontSize: '11px',
        color: 'var(--text-dim)',
        letterSpacing: '0.04em',
      }}>
        live request map
      </span>
      <LiveDot />
    </div>
  )
}

function LiveDot() {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
      <div style={{
        width: 6,
        height: 6,
        borderRadius: '50%',
        background: 'var(--green)',
        boxShadow: '0 0 6px var(--green)',
        animation: 'liveblink 2s ease-in-out infinite',
      }} />
      <style>{`
        @keyframes liveblink {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>
    </div>
  )
}

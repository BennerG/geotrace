import { useState, useEffect, useRef } from 'react'
import { StatsResponse } from '../types'

export function useStats(from: string, to: string, intervalMs = 15000) {
  const [stats, setStats] = useState<StatsResponse | null>(null)
  const [error, setError] = useState(false)
  const requestIdRef = useRef(0)

  useEffect(() => {
    let cancelled = false
    const thisRequestId = ++requestIdRef.current

    const fetchStats = async () => {
      try {
        const res = await fetch(`/stats?from=${from}&to=${to}`)
        if (!res.ok) throw new Error()
        const data = await res.json() as StatsResponse
        if (!cancelled && thisRequestId === requestIdRef.current) {
          setStats(data)
          setError(false)
        }
      } catch {
        if (!cancelled) setError(true)
      }
    }

    fetchStats()
    const id = setInterval(fetchStats, intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [from, to, intervalMs])

  return { stats, error }
}

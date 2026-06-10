import { useState, useEffect, useRef } from 'react'
import { StatsResponse } from '../types'

export function useStats(from: string, to: string, intervalMs = 15000) {
  const [stats, setStats] = useState<StatsResponse | null>(null)
  const [error, setError] = useState(false)
  const fromRef = useRef(from)
  const toRef = useRef(to)

  useEffect(() => {
    fromRef.current = from
    toRef.current = to
  }, [from, to])

  useEffect(() => {
    let cancelled = false

    const fetch_ = async () => {
      try {
        const res = await fetch(`/stats?from=${fromRef.current}&to=${toRef.current}`)
        if (!res.ok) throw new Error()
        const data = await res.json() as StatsResponse
        if (!cancelled) {
          setStats(data)
          setError(false)
        }
      } catch {
        if (!cancelled) setError(true)
      }
    }

    fetch_()
    const id = setInterval(fetch_, intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs])

  return { stats, error }
}

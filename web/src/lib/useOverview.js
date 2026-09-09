import { useCallback, useEffect, useRef, useState } from 'react'

const HOST = 'local'
const INTERVAL = 10000

// A failed poll must never blank the screen. The moment the daemon is shaky is
// the moment you most need to see the last known state.
export function useOverview() {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [fetchedAt, setFetchedAt] = useState(null)
  const [loading, setLoading] = useState(true)
  const timer = useRef(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch(`/api/hosts/${HOST}/overview`)
      if (!res.ok) throw new Error(`The daemon answered ${res.status}`)
      setData(await res.json())
      setFetchedAt(new Date())
      setError(null)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  return { data, error, fetchedAt, loading, reload: load }
}

export function useCommandMode() {
  const [enabled, setEnabled] = useState(() => {
    try {
      return localStorage.getItem('croft.commands') === 'true'
    } catch {
      return false
    }
  })

  const toggle = useCallback(() => {
    setEnabled((current) => {
      const next = !current
      try {
        localStorage.setItem('croft.commands', String(next))
      } catch {
        // A locked-down browser is not a reason to break the panel.
      }
      return next
    })
  }, [])

  return [enabled, toggle]
}

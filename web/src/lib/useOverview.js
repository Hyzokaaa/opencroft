import { useCallback, useEffect, useRef, useState } from 'react'

const HOST = 'local'
const INTERVAL = 10000

// A failed poll must never blank the screen. The moment the daemon is shaky is
// the moment you most need to see the last known state.
export function useOverview() {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [unauthorized, setUnauthorized] = useState(false)
  const [fetchedAt, setFetchedAt] = useState(null)
  const [loading, setLoading] = useState(true)
  const timer = useRef(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch(`/api/hosts/${HOST}/overview`)

      // A session can expire while the panel is open. Say so instead of
      // showing a generic failure over stale data.
      if (res.status === 401) {
        setUnauthorized(true)
        return
      }
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

  return { data, error, fetchedAt, loading, unauthorized, reload: load }
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

// Who is signed in, and whether anybody can be. The panel shell renders before
// this resolves, so the answer is three-state: unknown, in, out.
export function useAuth() {
  const [state, setState] = useState(null)

  const load = useCallback(async () => {
    try {
      const res = await fetch('/api/auth/state')
      setState(await res.json())
    } catch {
      setState({ hasUsers: true, authenticated: false, unreachable: true })
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const signOut = useCallback(async () => {
    await fetch('/api/auth/logout', { method: 'POST' })
    setState((current) => ({ ...current, authenticated: false }))
  }, [])

  return { auth: state, refreshAuth: load, signOut }
}

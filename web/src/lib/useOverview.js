import { useCallback, useEffect, useRef, useState } from 'react'
import { readJSON, humane, isAgentDown } from './api.js'

const HOST = 'local'
export const INTERVAL = 10000

// A failed poll must never blank the screen. The moment the daemon is shaky is
// the moment you most need to see the last known state.
//
// agentDown is the one failure worth naming apart: the panel answers, the
// agent behind it does not. It clears on the first good poll. host is where
// the server is, and the panel knows it even then — it is what somebody needs
// to go and look.
export function useOverview() {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [agentDown, setAgentDown] = useState(false)
  const [agentDetail, setAgentDetail] = useState(null)
  const [host, setHost] = useState(null)
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
      const payload = await readJSON(res)
      setData(payload)
      if (payload?.host) setHost(payload.host)
      setFetchedAt(new Date())
      setError(null)
      setAgentDown(false)
      setAgentDetail(null)
    } catch (e) {
      setError(humane(e))
      const down = isAgentDown(e)
      setAgentDown(down)
      setAgentDetail(down ? (e.detail ?? null) : null)
      if (down && e.host) setHost(e.host)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  return { data, error, agentDown, agentDetail, host, fetchedAt, loading, unauthorized, reload: load }
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
// this resolves, so the answer is three-state: unknown, in, out — and a fourth
// when nobody answered. That one used to pass for "out", and a person whose
// daemon was down was shown a sign-in form that could never work.
export function useAuth() {
  const [state, setState] = useState(null)

  const load = useCallback(async () => {
    try {
      const res = await fetch('/api/auth/state')
      const payload = await readJSON(res)
      // Something that answers without saying whether you are signed in is
      // not croft — a proxy's page, most often — and is not taken for "no".
      if (typeof payload?.authenticated !== 'boolean') {
        throw new Error('Something answered in place of croft, without saying who is signed in.')
      }
      setState(payload)
    } catch (e) {
      setState({ authenticated: false, unreachable: true, reason: humane(e) })
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

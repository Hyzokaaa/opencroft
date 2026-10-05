import { useCallback, useEffect, useRef, useState } from 'react'
import { readJSON, humane } from './api.js'

const HOST = 'local'
const INTERVAL = 5000

// What a container runs is read from the machine every few seconds, not
// remembered. A service that died between two glances should show as dead.
export function useServices(container) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [unavailable, setUnavailable] = useState(null)
  const [fetchedAt, setFetchedAt] = useState(null)
  const timer = useRef(null)

  const load = useCallback(async () => {
    if (!container) return
    try {
      const res = await fetch(`/api/hosts/${HOST}/instances/${container}/services`)
      setData(await readJSON(res))
      setFetchedAt(new Date())
      setError(null)
      setUnavailable(null)
    } catch (e) {
      // A host with no privileged side cannot look inside a container at all.
      // That is a fact about the host, not a read that failed, and trying
      // again changes nothing — so it is told apart from an error.
      if (e.status === 503) {
        setUnavailable(e.message)
        setError(null)
        return
      }
      setError(humane(e))
    }
  }, [container])

  useEffect(() => {
    // Another container is another subject. Without this the panel shows one
    // machine's services under another machine's name for as long as the next
    // read takes — correct data, attributed to the wrong host, which is the
    // worst thing an infrastructure panel can do.
    setData(null)
    setError(null)
    setUnavailable(null)
    setFetchedAt(null)

    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  // `read` is what separates "there is nothing here" from "I do not know yet".
  // Without it an empty array means both, and the panel asserts the first.
  return { data, error, unavailable, fetchedAt, read: data !== null, reload: load }
}

// A database lives inside the container that uses it, which is what makes a
// snapshot of that container a snapshot of the application and its data at the
// same instant. What comes back is metadata only: the password is generated on
// the privileged side and never crosses back.
export function useDatabases(container) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [unavailable, setUnavailable] = useState(null)
  const timer = useRef(null)

  const load = useCallback(async () => {
    if (!container) return
    try {
      const res = await fetch(`/api/hosts/${HOST}/instances/${container}/databases`)
      setData(await readJSON(res))
      setError(null)
      setUnavailable(null)
    } catch (e) {
      // A host with no privileged side says so rather than looking broken —
      // and rather than looking empty, which was the other lie on offer.
      if (e.status === 503) {
        setUnavailable(e.message)
        setError(null)
        return
      }
      setError(humane(e))
    }
  }, [container])

  useEffect(() => {
    // Another container is another subject, same as the services above.
    setData(null)
    setError(null)
    setUnavailable(null)

    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  return { data, error, unavailable, read: data !== null, reload: load }
}

// What runs inside the container as a database that croft has not written
// down — the one an install script left beside the application. Finding it
// means running a command in the container, so it is not read on a timer: once
// when the page opens, again after any database dialog, and when asked.
// A stopped container has nothing running to ask, so it is not asked: the
// look happens when it is running, and again when it starts.
export function useFoundDatabases(container, running = true) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [looking, setLooking] = useState(false)
  const [lookedAt, setLookedAt] = useState(null)
  const current = useRef(container)

  const load = useCallback(async () => {
    if (!container || !running) return
    setLooking(true)
    try {
      const res = await fetch(`/api/hosts/${HOST}/instances/${container}/databases/found`)
      const payload = await readJSON(res)
      // An answer about the container left behind is not about this one.
      if (current.current !== container) return
      setData(payload ?? [])
      setLookedAt(new Date())
      setError(null)
    } catch (e) {
      if (current.current !== container) return
      setError(humane(e))
    } finally {
      if (current.current === container) setLooking(false)
    }
  }, [container, running])

  useEffect(() => {
    current.current = container
    setData(null)
    setError(null)
    setLookedAt(null)
    setLooking(false)
    load()
  }, [container, load])

  return { data, error, looking, lookedAt, read: data !== null, reload: load }
}

// Following logs is polling, and saying so is better than implying a stream we
// do not have. journalctl is asked for the last N lines; the same lines coming
// back twice is cheap and the difference is invisible.
// kind picks the endpoint: a service croft deployed, or a unit it only found.
// The two live under different paths because a unit name carries no
// guarantee of the croft- prefix a service's does.
export function useLogs(container, service, { lines = 200, follow = false, kind = 'service' } = {}) {
  const [text, setText] = useState('')
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)
  const timer = useRef(null)

  const load = useCallback(async () => {
    if (!container || !service) return
    try {
      const segment = kind === 'unit' ? 'units' : 'services'
      const res = await fetch(
        `/api/hosts/${HOST}/instances/${container}/${segment}/${service}/logs?lines=${lines}`,
      )
      const payload = await readJSON(res)
      setText(payload?.lines ?? '')
      setError(null)
    } catch (e) {
      // The lines already read stay: a failed poll is said beside them, not
      // in place of them.
      setError(humane(e))
    } finally {
      setLoading(false)
    }
  }, [container, service, lines, kind])

  useEffect(() => {
    load()
    if (!follow) return

    timer.current = setInterval(load, 2000)
    return () => clearInterval(timer.current)
  }, [load, follow])

  return { text, error, loading, reload: load }
}

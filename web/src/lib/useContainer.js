import { useCallback, useEffect, useRef, useState } from 'react'

const HOST = 'local'
const INTERVAL = 5000

// What a container runs is read from the machine every few seconds, not
// remembered. A service that died between two glances should show as dead.
export function useServices(container) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [fetchedAt, setFetchedAt] = useState(null)
  const timer = useRef(null)

  const load = useCallback(async () => {
    if (!container) return
    try {
      const res = await fetch(`/api/hosts/${HOST}/instances/${container}/services`)
      const payload = await res.json()

      if (!res.ok) throw new Error(payload.error ?? `The daemon answered ${res.status}`)
      setData(payload)
      setFetchedAt(new Date())
      setError(null)
    } catch (e) {
      setError(e.message)
    }
  }, [container])

  useEffect(() => {
    // Another container is another subject. Without this the panel shows one
    // machine's services under another machine's name for as long as the next
    // read takes — correct data, attributed to the wrong host, which is the
    // worst thing an infrastructure panel can do.
    setData(null)
    setError(null)
    setFetchedAt(null)

    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  // `read` is what separates "there is nothing here" from "I do not know yet".
  // Without it an empty array means both, and the panel asserts the first.
  return { data, error, fetchedAt, read: data !== null, reload: load }
}

// Following logs is polling, and saying so is better than implying a stream we
// do not have. journalctl is asked for the last N lines; the same lines coming
// back twice is cheap and the difference is invisible.
export function useLogs(container, service, { lines = 200, follow = false } = {}) {
  const [text, setText] = useState('')
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)
  const timer = useRef(null)

  const load = useCallback(async () => {
    if (!container || !service) return
    try {
      const res = await fetch(
        `/api/hosts/${HOST}/instances/${container}/services/${service}/logs?lines=${lines}`,
      )
      const payload = await res.json()

      if (!res.ok) throw new Error(payload.error ?? `The daemon answered ${res.status}`)
      setText(payload.lines ?? '')
      setError(null)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [container, service, lines])

  useEffect(() => {
    load()
    if (!follow) return

    timer.current = setInterval(load, 2000)
    return () => clearInterval(timer.current)
  }, [load, follow])

  return { text, error, loading, reload: load }
}

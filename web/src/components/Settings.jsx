import { useEffect, useState } from 'react'
import Command from './Command.jsx'

const PROVIDERS = [
  { id: 'ovh', label: 'OVH' },
  { id: 'cloudflare', label: 'Cloudflare' },
]

// Credentials go one way only. They travel through the unprivileged half in
// memory and are written by the agent, root-only, mode 0600. Nothing here can
// read them back — the panel only ever learns *that* they exist.
export default function Settings() {
  const [status, setStatus] = useState(null)
  const [provider, setProvider] = useState('ovh')
  const [values, setValues] = useState({})
  const [error, setError] = useState(null)
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const res = await fetch('/api/hosts/local/dns')
      const payload = await res.json()
      setStatus(payload)
      if (payload.provider) setProvider(payload.provider)
    } catch (e) {
      setError(e.message)
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function save(event) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    setSaved(false)

    try {
      const res = await fetch('/api/hosts/local/dns', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, values }),
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        setError(payload.error ?? `The daemon answered ${res.status}`)
        return
      }
      setValues({})
      setSaved(true)
      load()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const keys = keysFor(provider, status)

  return (
    <div className="max-w-2xl space-y-4">
      <section className="rounded-lg border border-edge bg-panel">
        <header className="border-b border-edge px-4 py-3">
          <h2 className="text-sm font-medium">DNS credentials</h2>
          <p className="mt-1 text-xs text-muted">
            Only needed for the DNS challenge — for wildcards, or when port 80 cannot be
            reached from the internet. Ordinary certificates need none of this.
          </p>
        </header>

        <div className="space-y-4 px-4 py-4">
          {status?.configured ? (
            <div className="rounded border border-edge bg-ground px-3 py-2.5">
              <p className="text-sm">
                <span className="text-running">✓</span> {status.provider} is configured
              </p>
              <p className="mt-1 font-mono text-xs text-faint">read from {status.source}</p>
              <p className="mt-2 text-xs text-muted">
                The values are not shown here and cannot be read back. Fill the form to
                replace them.
              </p>
            </div>
          ) : (
            <p className="text-sm text-muted">Nothing configured.</p>
          )}

          <form onSubmit={save} className="space-y-3">
            <label className="block">
              <span className="mb-1 block text-xs text-muted">Provider</span>
              <select
                value={provider}
                onChange={(e) => {
                  setProvider(e.target.value)
                  setValues({})
                }}
                className="w-full rounded border border-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-edge-strong"
              >
                {PROVIDERS.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.label}
                  </option>
                ))}
              </select>
            </label>

            {keys.map((key) => (
              <label key={key} className="block">
                <span className="mb-1 block font-mono text-xs text-muted">{key}</span>
                <input
                  type="password"
                  autoComplete="off"
                  value={values[key] ?? ''}
                  onChange={(e) => setValues({ ...values, [key]: e.target.value })}
                  className="w-full rounded border border-edge bg-ground px-3 py-2 font-mono text-sm outline-none transition focus:border-edge-strong"
                />
              </label>
            ))}

            {error && (
              <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
                {error}
              </p>
            )}
            {saved && <p className="text-xs text-running">Stored on the host, readable only by root.</p>}

            <button
              type="submit"
              disabled={busy || keys.every((k) => !values[k])}
              className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
            >
              {busy ? 'Storing…' : 'Store credentials'}
            </button>
          </form>
        </div>

        <footer className="border-t border-edge px-4 py-3">
          <p className="mb-2 text-[11px] uppercase tracking-wide text-faint">The same thing, by hand</p>
          <Command
            lines={[
              '# croft reads certbot\'s files too, if you already have them',
              'sudo croft dns set ovh',
              'sudo croft dns show',
            ]}
          />
        </footer>
      </section>
    </div>
  )
}

function keysFor(provider, status) {
  if (status?.provider === provider && status.keys?.length) return status.keys
  return provider === 'cloudflare'
    ? ['cloudflare_api_token']
    : ['ovh_endpoint', 'ovh_application_key', 'ovh_application_secret', 'ovh_consumer_key']
}

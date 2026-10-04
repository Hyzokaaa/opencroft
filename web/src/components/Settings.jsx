import { useEffect, useId, useState } from 'react'
import Command from './Command.jsx'
import { readJSON, humane } from '../lib/api.js'

const PROVIDERS = [
  { id: 'ovh', label: 'OVH' },
  { id: 'cloudflare', label: 'Cloudflare' },
]

// Credentials go one way only. They travel through the unprivileged half in
// memory and are written by the agent, root-only, mode 0600. Nothing here can
// read them back — the panel only ever learns *that* they exist.
export default function Settings({ onExpose }) {
  const [status, setStatus] = useState(null)
  const [loadError, setLoadError] = useState(null)
  const [provider, setProvider] = useState('ovh')
  const [values, setValues] = useState({})
  const [error, setError] = useState(null)
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const payload = await readJSON(await fetch('/api/hosts/local/dns'))
      setStatus(payload ?? {})
      setLoadError(null)
      if (payload?.provider) setProvider(payload.provider)
    } catch (e) {
      setLoadError(humane(e))
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
      await readJSON(res)
      setValues({})
      setSaved(true)
      load()
    } catch (e) {
      setError(humane(e))
    } finally {
      setBusy(false)
    }
  }

  const keys = keysFor(provider, status)
  // Every key a provider needs, or none stored: half a set of credentials
  // fails at the first certificate, far from here.
  const complete = keys.every((k) => values[k]?.trim())

  return (
    <div className="max-w-2xl space-y-4">
      <PanelAddress onExpose={onExpose} />

      <section className="rounded-lg border border-edge bg-panel">
        <header className="border-b border-edge px-4 py-3">
          <h2 className="text-sm font-medium">DNS credentials</h2>
          <p className="mt-1 text-xs text-muted">
            Only needed for the DNS challenge — for wildcards, or when port 80 cannot be
            reached from the internet. Ordinary certificates need none of this.
          </p>
        </header>

        <div className="space-y-4 px-4 py-4">
          {loadError ? (
            <p role="alert" className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
              Could not read whether credentials are stored. {loadError}
            </p>
          ) : !status ? (
            <p role="status" className="text-sm text-muted">Reading&hellip;</p>
          ) : status.configured ? (
            <div className="rounded border border-edge bg-ground px-3 py-2.5">
              {/* Green says "running" on this panel and nothing else. */}
              <p className="text-sm">
                <span className="text-muted">✓</span> {status.provider} is configured
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
                className="w-full rounded border border-field-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-ink/40"
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
                  className="w-full rounded border border-field-edge bg-ground px-3 py-2 font-mono text-sm outline-none transition focus:border-ink/40"
                />
              </label>
            ))}

            {/* Unlike everything else on this panel, this is not shown as a
                plan first: a plan is read on screen, and a secret should not
                be. It goes straight to the host. */}
            <p className="text-xs text-muted">
              Stored directly when you press Store, without a plan to read first — a plan would put
              the secrets on screen.
            </p>

            {error && (
              <p role="alert" className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
                {error}
              </p>
            )}
            {saved && (
              <p role="status" className="text-xs text-ink">
                <span className="text-muted">✓</span> Stored on the host, readable only by root.
              </p>
            )}

            <button
              type="submit"
              disabled={busy || !complete}
              className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
            >
              {busy ? 'Storing…' : 'Store credentials'}
            </button>
            {!complete && keys.length > 1 && (
              <span className="ml-3 text-xs text-muted">All {keys.length} are needed.</span>
            )}
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

// Exposing the panel from the panel is worth a word of warning: if the vhost
// were rejected, the thing you are using to do this is what would break. It
// is removed again in that case, so the worst outcome is that nothing changed.
function PanelAddress({ onExpose }) {
  const [domain, setDomain] = useState('')
  const field = useId()

  return (
    <section className="rounded-lg border border-edge bg-panel">
      <header className="border-b border-edge px-4 py-3">
        <h2 className="text-sm font-medium">Panel address</h2>
        <p className="mt-1 text-xs text-muted">
          Give this panel a domain and reach it over https, instead of an SSH tunnel.
        </p>
      </header>

      <div className="space-y-3 px-4 py-4">
        <p className="text-xs text-muted">
          The domain must already point at this server. The panel keeps listening on
          localhost — nginx is what the internet reaches, and it terminates TLS.
        </p>

        <form
          onSubmit={(e) => {
            e.preventDefault()
            onExpose?.(domain.trim())
          }}
          className="space-y-1"
        >
          <label htmlFor={field} className="block text-xs text-muted">
            Domain
          </label>
          <div className="flex gap-2">
          <input
            id={field}
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            placeholder="panel.example.com"
            className="min-w-0 flex-1 rounded border border-field-edge bg-ground px-3 py-2 font-mono text-sm outline-none transition focus:border-ink/40"
          />
          <button
            type="submit"
            disabled={!domain.trim()}
            className="shrink-0 rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
          >
            Show me the plan
          </button>
          </div>
        </form>
      </div>

      <footer className="border-t border-edge px-4 py-3">
        <p className="mb-2 text-[11px] uppercase tracking-wide text-faint">The same thing, by hand</p>
        <Command lines={['sudo croft expose panel.example.com']} />
      </footer>
    </section>
  )
}

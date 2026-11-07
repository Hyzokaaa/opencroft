import { useEffect, useId, useState } from 'react'
import Command from './Command.jsx'
import CopyButton from './CopyButton.jsx'
import { PageHead, Tabs } from './Listing.jsx'
import { href } from '../lib/useRoute.js'
import { PRIMARY } from '../lib/ui.js'
import { readJSON, humane } from '../lib/api.js'

const PROVIDERS = [
  { id: 'ovh', label: 'OVH' },
  { id: 'cloudflare', label: 'Cloudflare' },
]

// One section per screen, each with its own address and its one button. On
// one page, the panel's address and the DNS credentials each had a solid
// button and neither was the page's.
const PARTS = [
  { id: 'panel', label: 'Panel address' },
  { id: 'dns', label: 'DNS credentials' },
  { id: 'about', label: 'About this host' },
]

export default function Settings({ part: asked, data, host, onExpose }) {
  const part = PARTS.some((p) => p.id === asked) ? asked : 'panel'
  return (
    <>
      <PageHead title="Settings" summary="This host's panel, and what croft needs to get certificates for it." />
      <Tabs parts={PARTS} part={part} to={(p) => href('settings', p === 'panel' ? null : p)} label="Settings: sections" />
      <div className="max-w-2xl">
        {part === 'panel' && <PanelAddress onExpose={onExpose} />}
        {part === 'dns' && <DnsCredentials />}
        {part === 'about' && <About data={data} host={host} />}
      </div>
    </>
  )
}

// What somebody is asked for when they report a problem, or wants to know
// whether to update: which croft, on what, reached where. The panel and its
// agent are the same binary — the panel refuses an agent of another version —
// so one number answers for both.
function About({ data, host }) {
  const addresses = host?.addresses ?? []
  const facts = [
    ['Version', data?.version ?? 'dev'],
    ['Runs containers with', data?.runtime ?? '—'],
    ['Containers', String(data?.instances?.length ?? 0)],
  ]
  return (
    <section className="rounded-lg border border-edge bg-panel">
      <header className="border-b border-edge px-4 py-3">
        <h2 className="text-sm font-medium">About this host</h2>
        <p className="mt-1 text-xs text-muted">
          The panel and the agent that acts for it as root are one binary, so they are always the same version.
        </p>
      </header>
      <dl className="grid gap-x-6 gap-y-3 px-4 py-4 sm:grid-cols-2">
        {facts.map(([label, value]) => (
          <div key={label}>
            <dt className="text-[11px] text-muted">{label}</dt>
            <dd className="mt-0.5 font-mono text-sm">{value}</dd>
          </div>
        ))}
        <div className="sm:col-span-2">
          <dt className="text-[11px] text-muted">Addresses</dt>
          <dd className="mt-0.5 space-y-1">
            {addresses.length === 0 ? (
              <span className="text-xs text-muted">None read.</span>
            ) : (
              addresses.map((a) => (
                <p key={a.address} className="flex items-center gap-2">
                  <span className="font-mono text-sm">{a.address}</span>
                  <span className="text-xs text-muted">{a.public ? 'public' : 'private — reachable only from its own network'}</span>
                  <CopyButton text={a.address} label={`Copy ${a.address}`} />
                </p>
              ))
            )}
          </dd>
        </div>
        {data?.demo && (
          <p className="text-xs text-yours sm:col-span-2">This is the demo: what it shows is made up, and nothing it does reaches a machine.</p>
        )}
      </dl>
      <footer className="border-t border-edge px-4 py-3">
        <p className="mb-2 text-[11px] uppercase tracking-wide text-faint">Update it</p>
        <p className="mb-2 text-xs text-muted">
          Downloads the newest release and restarts the agent, then the panel. Run on the host; the first line only
          says whether there is a newer one.
        </p>
        <Command lines={['croft update --check', 'sudo croft update']} />
        <a
          href="https://github.com/Hyzokaaa/opencroft/releases"
          target="_blank"
          rel="noreferrer"
          className="mt-2 inline-block text-xs text-muted underline underline-offset-4 hover:text-ink"
        >
          What each release changed
        </a>
      </footer>
    </section>
  )
}

// Credentials go one way only. They travel through the unprivileged half in
// memory and are written by the agent, root-only, mode 0600. Nothing here can
// read them back — the panel only ever learns *that* they exist.
function DnsCredentials() {
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
            className={PRIMARY}
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
  // Said when the button is pressed with nothing typed, rather than a dimmed
  // button that gives no reason for not working.
  const [empty, setEmpty] = useState(false)
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
            if (!domain.trim()) return setEmpty(true)
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
            onChange={(e) => {
              setDomain(e.target.value)
              setEmpty(false)
            }}
            aria-invalid={empty || undefined}
            placeholder="panel.example.com"
            className="min-w-0 flex-1 rounded border border-field-edge bg-ground px-3 py-2 font-mono text-sm outline-none transition focus:border-ink/40"
          />
          <button
            type="submit"
            className={`shrink-0 ${PRIMARY}`}
          >
            Show me the plan
          </button>
          </div>
          {empty && <p role="alert" className="text-xs text-caution">Type the domain the panel should answer at first.</p>}
        </form>
      </div>

      <footer className="border-t border-edge px-4 py-3">
        <p className="mb-2 text-[11px] uppercase tracking-wide text-faint">The same thing, by hand</p>
        <Command lines={['sudo croft expose panel.example.com']} />
      </footer>
    </section>
  )
}

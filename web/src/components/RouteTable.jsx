import Chip from './Chip.jsx'
import Actions from './Actions.jsx'
import { OWNERSHIP, VERBS } from '../lib/vocabulary.js'
import { href } from '../lib/useRoute.js'

export default function RouteTable({ routes, instances, certificates = [], problems, highlighted, onHover, onRemoveDomain, onEnableTLS, onEditDomain, onAddPath, onRemovePath, onTakeOver, onAddAlias, onRetryAlias, onRemoveAlias, empty }) {
  if (!routes.length) {
    return <p className="px-4 py-8 text-center text-sm text-muted">{empty ?? 'No domains routed yet.'}</p>
  }

  const nameFor = (address) => instances.find((i) => i.address === address)?.name

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
            <th className="px-4 py-2 font-medium">Domain</th>
            <th className="hidden px-4 py-2 font-medium sm:table-cell">Serves</th>
            <th className="hidden px-4 py-2 font-medium md:table-cell">Certificate</th>
            <th className="hidden px-4 py-2 font-medium xl:table-cell">File</th>
            <th className="px-4 py-2"><span className="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {routes.map((r) => {
            const target = nameFor(r.target)
            const ownership = OWNERSHIP[r.state] ?? OWNERSHIP.managed
            const problem = problems.has(r.domain)
            const lit = highlighted === r.domain || (target && highlighted === target)
            // Opening a domain means opening what it serves: that is where
            // anything about it gets fixed.
            const opens = target ? href('containers', target) : null

            return (
              <tr
                key={r.domain}
                onMouseEnter={() => onHover?.(target ?? r.domain)}
                onMouseLeave={() => onHover?.(null)}
                onClick={() => opens && (location.hash = opens)}
                className={`border-b border-edge/50 transition-colors last:border-0 ${opens ? 'cursor-pointer' : ''} ${
                  lit ? 'bg-white/[0.05]' : 'hover:bg-white/[0.03]'
                }`}
              >
                <td className="relative px-4 py-2.5">
                  {problem && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
                  <div className="flex items-center gap-2">
                    {r.ssl ? <Lock /> : <span className="w-3 shrink-0" />}
                    {opens ? (
                      <a href={opens} onClick={(e) => e.stopPropagation()} className="font-mono underline-offset-4 hover:underline">
                        {r.domain}
                      </a>
                    ) : (
                      <span className="font-mono">{r.domain}</span>
                    )}
                    {r.state !== 'managed' && (
                      <Chip label={ownership.label} tone={ownership.tone} explain={ownership.explain} />
                    )}
                  </div>
                  {/* Other names answering exactly like it — a customer's own
                      domain — under the one they stand in for. */}
                  {(r.aliases ?? []).map((a) => (
                    <div key={a.domain} className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 pl-5 text-xs">
                      <span className="text-faint">also</span>
                      <span className="flex items-center gap-1.5">
                        {a.ssl && <Lock />}
                        <span className="font-mono">{a.domain}</span>
                      </span>
                      <AliasState route={r} alias={a} />
                      {r.state === 'managed' && (
                        <span className="flex gap-1">
                          {aliasActions(r, a, { onRetryAlias, onRemoveAlias }).filter(Boolean).map((act) => (
                            <button
                              key={act.label}
                              onClick={(e) => { e.stopPropagation(); act.onClick() }}
                              aria-label={act.name}
                              className={`min-h-6 rounded px-1.5 text-faint transition ${act.danger ? 'hover:text-problem' : 'hover:text-ink'}`}
                            >
                              {act.label}
                            </button>
                          ))}
                        </span>
                      )}
                    </div>
                  ))}
                  {/* A prefix served from somewhere of its own, under the domain
                      it belongs to — that is where a person looks for /api/. */}
                  {(r.paths ?? []).map((p) => (
                    <div key={p.prefix} className="mt-1 flex flex-wrap items-center gap-2 pl-5 text-xs">
                      <span className="font-mono text-muted">{p.prefix}</span>
                      <span className="text-faint" aria-hidden="true">→</span>
                      <span className="font-mono">{nameFor(p.target) ?? p.target}</span>
                      <span className="text-faint">:{p.port}</span>
                      {p.strip && <span className="text-faint">without {p.prefix}</span>}
                      {r.state === 'managed' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); onRemovePath?.(r, p.prefix) }}
                          aria-label={`Stop sending ${p.prefix} elsewhere`}
                          className="rounded px-1 text-faint transition hover:text-problem"
                        >
                          ✕
                        </button>
                      )}
                    </div>
                  ))}
                </td>

                <td className="hidden px-4 py-2.5 text-xs sm:table-cell">
                  <span className="font-mono">{target ?? r.target}</span>
                  <span className="text-faint">:{r.port}</span>
                  {/* Configured correctly and still returning 502 is a real
                      state, and the one a visitor notices first. */}
                  {r.answers === false && (
                    <span className="ml-2 text-problem">nothing listening</span>
                  )}
                </td>

                <td className="hidden px-4 py-2.5 text-xs md:table-cell">
                  <Expiry route={r} certificates={certificates} />
                </td>

                <td className="hidden px-4 py-2.5 font-mono text-xs text-faint xl:table-cell">{r.file}</td>

                <td className="px-4 py-2.5 text-right">
                  <Actions
                    label={`Actions for ${r.domain}`}
                    actions={routeActions(r, { onRemoveDomain, onEnableTLS, onEditDomain, onAddPath, onTakeOver, onAddAlias })}
                  />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// routeActions is what can be done to a domain, wherever it is listed — here
// and on the page of the container it serves — so the two never disagree.
export function routeActions(r, { onRemoveDomain, onEnableTLS, onEditDomain, onAddPath, onTakeOver, onAddAlias }) {
  // A vhost croft did not write is not croft's to edit or remove. What is on
  // offer is taking it over — explicitly, with a plan, the original moved
  // aside rather than lost.
  if (r.state === 'unmanaged') {
    return [{ label: `${VERBS.takeOver}…`, onClick: () => onTakeOver?.(r.domain) }]
  }
  return [
    { label: 'Edit', onClick: () => onEditDomain?.(r) },
    r.state === 'managed' && { label: 'Add path', onClick: () => onAddPath?.(r) },
    // Another name for this same domain — not Add domain, which is a new one.
    r.state === 'managed' && { label: 'Add name', onClick: () => onAddAlias?.(r) },
    !r.ssl && { label: 'Enable https', onClick: () => onEnableTLS?.(r) },
    // On https already, but renewed by certbot: two programs owning one
    // domain is how renewal breaks quietly when the DNS moves.
    r.ssl && r.state === 'managed' && r.certificates?.startsWith('/etc/letsencrypt/live/') &&
      { label: 'Issue with croft', onClick: () => onEnableTLS?.(r) },
    { label: 'Remove…', onClick: () => onRemoveDomain?.(r.domain), danger: true },
  ]
}

// aliasActions is what can be done to one other name of a domain, here and on
// the container's page. Retrying https is adding it again: the certificate it
// could not get then is asked for once more.
export function aliasActions(r, a, { onRetryAlias, onRemoveAlias }) {
  return [
    r.ssl && !a.ssl && { label: 'Retry https…', name: `Retry https for ${a.domain}`, onClick: () => onRetryAlias?.(r, a.domain) },
    { label: 'Remove…', name: `Remove ${a.domain}`, onClick: () => onRemoveAlias?.(r, a.domain), danger: true },
  ]
}

// AliasState says how an other name is served. On an https domain, one still
// on http is waiting for a certificate — nearly always for its DNS to point
// here — and says so in words, not only by the missing lock.
export function AliasState({ route, alias }) {
  if (alias.ssl || !route.ssl) return null
  return <span className="text-caution">http only — waiting for its certificate</span>
}

// The certificate a domain is served with, and how long it has: the deadline
// belongs where the domain is managed, not only on a page of its own.
function Expiry({ route, certificates }) {
  if (!route.ssl) return <span className="text-faint">http only</span>
  const covering = certificates.find(
    (c) => c.domain === route.domain || (c.names ?? []).some((n) => n === route.domain || covers(n, route.domain)),
  )
  if (!covering) return <span className="text-faint">&mdash;</span>
  const tone = covering.daysLeft < 7 ? 'text-problem' : covering.daysLeft < 21 ? 'text-caution' : 'text-muted'
  return (
    <span className={tone}>
      {covering.daysLeft < 0 ? 'expired' : `${covering.daysLeft} days`}
      {covering.domain.startsWith('*.') && <span className="text-faint"> · wildcard</span>}
    </span>
  )
}

function covers(name, domain) {
  if (!name.startsWith('*.')) return false
  const base = name.slice(2)
  const label = domain.slice(0, -(base.length + 1))
  return domain.endsWith('.' + base) && label !== '' && !label.includes('.')
}

// TLS is a property, not a state — it gets an icon, not the green that means
// "running".
function Lock() {
  return (
    <svg viewBox="0 0 16 16" className="size-3 shrink-0 text-muted" fill="currentColor" role="img" aria-label="https">
      <path d="M8 1a3 3 0 0 0-3 3v2H4.5A1.5 1.5 0 0 0 3 7.5v6A1.5 1.5 0 0 0 4.5 15h7a1.5 1.5 0 0 0 1.5-1.5v-6A1.5 1.5 0 0 0 11.5 6H11V4a3 3 0 0 0-3-3Zm0 1.5A1.5 1.5 0 0 1 9.5 4v2h-3V4A1.5 1.5 0 0 1 8 2.5Z" />
    </svg>
  )
}

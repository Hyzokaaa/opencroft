import Chip from './Chip.jsx'
import Actions from './Actions.jsx'
import { OWNERSHIP, VERBS } from '../lib/vocabulary.js'
import { href } from '../lib/useRoute.js'
import { allNames, stateOf, certificateFor } from '../lib/domains.js'
import { nameActions } from './DomainList.jsx'

// Every name on the host, one row each. A route answering on three names is
// three things a visitor types, and each can be in a different state — one
// waiting for its DNS while the others serve — so each gets its own row, its
// own state and its own certificate. Which names were set up together is said
// on the row, not by nesting it.
export default function RouteTable({ routes, instances, certificates = [], problems, highlighted, onHover, empty, ...handlers }) {
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
            <th className="px-4 py-2 font-medium">State</th>
            <th className="hidden px-4 py-2 font-medium md:table-cell">Certificate</th>
            <th className="hidden px-4 py-2 font-medium xl:table-cell">File</th>
            <th className="px-4 py-2"><span className="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {allNames(routes).map((name) => {
            const r = name.route
            const target = nameFor(r.target)
            const ownership = OWNERSHIP[r.state] ?? OWNERSHIP.managed
            const problem = problems.has(name.domain)
            const lit = highlighted === name.domain || (target && highlighted === target)
            const state = stateOf(name, certificates)
            // Opening a domain means opening what it serves: that is where
            // anything about it gets fixed.
            const opens = target ? href('containers', target) : null

            return (
              <tr
                key={name.key}
                onMouseEnter={() => onHover?.(target ?? name.domain)}
                onMouseLeave={() => onHover?.(null)}
                onClick={() => opens && (location.hash = opens)}
                className={`border-b border-edge/50 transition-colors last:border-0 ${opens ? 'cursor-pointer' : ''} ${
                  lit ? 'bg-white/[0.05]' : 'hover:bg-white/[0.03]'
                }`}
              >
                <td className="relative px-4 py-2.5">
                  {problem && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
                  {/* The lock stays beside the name; the chip may wrap under
                      it on a narrow screen. */}
                  <div className="flex items-center gap-2">
                    {name.ssl ? <Lock /> : <span className="w-3 shrink-0" />}
                    <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                      {opens ? (
                        <a href={opens} onClick={(e) => e.stopPropagation()} className="break-words font-mono underline-offset-4 hover:underline">
                          {name.domain}
                        </a>
                      ) : (
                        <span className="break-words font-mono">{name.domain}</span>
                      )}
                      {r.state !== 'managed' && (
                        <Chip label={ownership.label} tone={ownership.tone} explain={ownership.explain} />
                      )}
                    </span>
                  </div>
                  {name.alias && (
                    <p className="mt-0.5 pl-5 text-[11px] text-faint">set up with {r.domain}</p>
                  )}
                  {/* A prefix served from somewhere of its own, under the
                      domain it belongs to — that is where a person looks for
                      /api/. */}
                  {name.entry && (r.paths ?? []).map((p) => (
                    <div key={p.prefix} className="mt-1 flex flex-wrap items-center gap-2 pl-5 text-xs">
                      <span className="font-mono text-muted">{p.prefix}</span>
                      <span className="text-faint" aria-hidden="true">→</span>
                      <span className="font-mono">{nameFor(p.target) ?? p.target}</span>
                      <span className="text-faint">:{p.port}</span>
                      {p.strip && <span className="text-faint">without {p.prefix}</span>}
                      {r.state === 'managed' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handlers.onRemovePath?.(r, p.prefix) }}
                          aria-label={`Stop sending ${p.prefix} elsewhere`}
                          className="min-h-6 rounded px-1 text-faint transition hover:text-problem"
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
                </td>

                <td className="whitespace-nowrap px-4 py-2.5 text-xs">
                  {state.reason ? (
                    <Chip label={state.label} tone={`${BORDER[state.tone] ?? 'border-edge'} ${state.tone}`} explain={state.reason} />
                  ) : (
                    <span className={state.tone}>{state.label}</span>
                  )}
                </td>

                <td className="hidden px-4 py-2.5 text-xs md:table-cell">
                  <Expiry name={name} certificates={certificates} />
                </td>

                <td className="hidden px-4 py-2.5 font-mono text-xs text-faint xl:table-cell">{r.file}</td>

                <td className="px-4 py-2.5 text-right">
                  <Actions label={`Actions for ${name.domain}`} actions={nameActions(name, state, handlers)} />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// The border that goes with each state's text, for the chip that carries a
// reason.
const BORDER = {
  'text-problem': 'border-problem/40',
  'text-caution': 'border-caution/40',
  'text-muted': 'border-edge',
}

// routeActions is what can be done to a domain, wherever it is listed — here,
// on the page of the container it serves and on the service's — so they never
// disagree.
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
    // Another name for this same domain. A domain is given to a service, on
    // its page, and joins the route that already reaches it; this is only
    // offered where no service croft deployed is there to be given one.
    r.state === 'managed' && onAddAlias && { label: 'Add name…', onClick: () => onAddAlias(r) },
    !r.ssl && { label: 'Serve over https…', onClick: () => onEnableTLS?.(r) },
    // On https already, but renewed by certbot: two programs owning one
    // domain is how renewal breaks quietly when the DNS moves.
    r.ssl && r.state === 'managed' && r.certificates?.startsWith('/etc/letsencrypt/live/') &&
      { label: 'Issue with croft', onClick: () => onEnableTLS?.(r) },
    { label: 'Remove…', onClick: () => onRemoveDomain?.(r.domain), danger: true },
  ]
}

// The certificate a name is served with, and how long it has: the deadline
// belongs where the domain is managed, not only on a page of its own.
function Expiry({ name, certificates }) {
  if (!name.ssl) return <span className="text-faint">&mdash;</span>
  const covering = certificateFor(name.domain, certificates)
  if (!covering) return <span className="text-faint">&mdash;</span>
  const tone = covering.daysLeft < 7 ? 'text-problem' : covering.daysLeft < 21 ? 'text-caution' : 'text-muted'
  return (
    <span className={tone}>
      {covering.daysLeft < 0 ? 'expired' : `${covering.daysLeft} days`}
      {covering.domain.startsWith('*.') && <span className="text-faint"> · wildcard</span>}
    </span>
  )
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

import Chip from './Chip.jsx'
import Actions from './Actions.jsx'
import { OWNERSHIP } from '../lib/vocabulary.js'
import { stateOf } from '../lib/domains.js'
import { routeActions } from './RouteTable.jsx'

// The names reaching something, one per row, each saying how it is served in
// a word — and, when croft is waiting on somebody else's DNS, why and what to
// do about it. The same rows on a container's page and on a service's, so the
// two never disagree about a name.
//
//   names:    from lib/domains.js — { key, domain, ssl, port, route, alias?, prefix?, entry? }
//   handlers: the route handlers App gives every page; onAddAlias only where
//             a name can be added to the route itself (nothing else leads
//             there that a domain could be given to)
//   ports:    say where each one leads, when that is the point
export default function DomainList({ names, certificates, handlers, publicAddress, ports = false }) {
  return (
    <ul className="divide-y divide-edge">
      {names.map((name) => (
        <DomainRow
          key={name.key}
          name={name}
          state={stateOf(name, certificates)}
          handlers={handlers}
          publicAddress={publicAddress}
          port={ports}
        />
      ))}
    </ul>
  )
}

function DomainRow({ name, state, handlers, publicAddress, port }) {
  const { route, alias, prefix } = name
  const ownership = route.state !== 'managed' ? OWNERSHIP[route.state] : null
  const url = `${name.ssl ? 'https' : 'http'}://${name.domain}`

  return (
    <li className="flex flex-wrap items-start justify-between gap-2 px-4 py-2.5">
      <div className="min-w-0">
        <p className="flex flex-wrap items-center gap-2">
          {/* The name opens the site itself: checking it answers is the
              first thing done after looking at it here. */}
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            className="break-all font-mono text-xs underline-offset-4 hover:underline"
          >
            <span className="text-muted">{name.ssl ? 'https://' : 'http://'}</span>
            {name.domain}
            <span className="sr-only"> (opens in a new tab)</span>
          </a>
          {ownership && <Chip label={ownership.label} tone={ownership.tone} explain={ownership.explain} />}
        </p>
        <p className="mt-0.5 text-[11px]">
          <span className={state.tone}>{state.label}</span>
          {port && <span className="font-mono text-faint"> &middot; to :{name.port}</span>}
          {alias && <span className="text-faint"> &middot; set up with {route.domain}</span>}
          {prefix && <span className="text-faint"> &middot; a path of {route.domain}</span>}
        </p>
        {/* Waiting is not failing: croft asks again on its own. What it is
            waiting for is somebody else's DNS, so the row says what record,
            and where it has to point. */}
        {state.key === 'waiting' && (
          <p className="mt-1 max-w-prose text-[11px] text-muted">
            {sentence(state.reason)}{publicAddress &&<> This server is at <span className="font-mono text-ink">{publicAddress}</span>.</>}{' '}
            Croft checks again every 5 minutes.
          </p>
        )}
        {state.key === 'down' && <p className="mt-1 max-w-prose text-[11px] text-muted">{state.reason}</p>}
        {state.key === 'http' && state.reason && <p className="mt-1 text-[11px] text-muted">{state.reason}</p>}
      </div>

      <Actions label={`Actions for ${name.domain}`} actions={nameActions(name, state, handlers)} />
    </li>
  )
}

// The daemon's reasons come without a full stop, and sit before sentences of
// our own.
function sentence(text) {
  return /[.!?]$/.test(text) ? text : `${text}.`
}

// nameActions is what can be done to one name. Its own domain carries the
// route's actions; another name only its own certificate and its removal;
// a path, sending it back. Removing a route's own domain is as safe as any
// other: its other names stay, one of them taking its place.
export function nameActions(name, state, handlers) {
  const { route, alias, prefix } = name
  if (prefix) {
    return route.state === 'managed'
      ? [{ label: 'Stop sending it here…', onClick: () => handlers.onRemovePath?.(route, prefix) }]
      : []
  }
  if (alias) {
    if (route.state !== 'managed') return []
    return [
      state.key === 'waiting'
        ? { label: 'Check now…', onClick: () => handlers.onRetryAlias?.(route, alias.domain) }
        : route.ssl && !alias.ssl && { label: 'Serve over https…', onClick: () => handlers.onRetryAlias?.(route, alias.domain) },
      { label: 'Remove…', onClick: () => handlers.onRemoveAlias?.(route, alias.domain), danger: true },
    ]
  }
  return routeActions(route, handlers)
}

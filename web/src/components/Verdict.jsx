import { href } from '../lib/useRoute.js'

// One headline, not four decorative counters. The counters that remain are
// filters: each opens the list it counts, already narrowed to it. A number you
// cannot act on is wallpaper.
export default function Verdict({ data, problemCount }) {
  const running = data.instances.filter((i) => i.status === 'running').length
  const stopped = data.instances.length - running
  const secured = data.routes.filter((r) => r.ssl).length

  // An empty host is not "All good" — nothing is not working because nothing
  // is there. Said plainly, and without counters that would all read 0.
  if (data.instances.length === 0 && problemCount === 0) {
    return (
      <div className="rounded-lg border border-edge bg-panel px-5 py-4">
        <span className="text-2xl font-medium">Nothing here yet</span>
      </div>
    )
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-edge bg-panel px-5 py-4">
      <div className="flex items-center gap-3">
        {problemCount > 0 ? (
          <>
            <span className="flex size-6 items-center justify-center rounded-full border border-problem font-mono text-xs font-bold text-problem">
              !
            </span>
            <span className="text-2xl font-medium text-problem">
              {problemCount} problem{problemCount === 1 ? '' : 's'}
            </span>
          </>
        ) : (
          <>
            <span className="size-2.5 rounded-full bg-running" />
            <span className="text-2xl font-medium">All good</span>
          </>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-1 text-xs">
        <Pill label="containers" value={data.instances.length} to={href('containers')} />
        <Pill label="running" value={running} to={href('containers', null, { status: 'running' })} />
        {stopped > 0 && <Pill label="stopped" value={stopped} to={href('containers', null, { status: 'stopped' })} />}
        <Pill label="domains" value={data.routes.length} to={href('domains')} />
        <Pill label="with TLS" value={secured} to={href('domains', null, { tls: 'yes' })} />
      </div>
    </div>
  )
}

function Pill({ label, value, to }) {
  return (
    <a
      href={to}
      className="rounded border border-edge px-2.5 py-1.5 transition hover:border-edge-strong hover:bg-white/[0.03]"
    >
      <span className="font-medium">{value}</span> <span className="text-muted">{label}</span>
    </a>
  )
}

// StartHere is the home page of a host with no containers: the order things
// happen in, and the first of them as the one button that stands out. Without
// it, the first screen offered a project — which is a group of containers
// that do not exist yet.
export function StartHere({ onNewContainer }) {
  return (
    <section aria-labelledby="start-here" className="rounded-lg border border-edge bg-panel px-5 py-4">
      <h2 id="start-here" className="text-sm font-medium">Start here</h2>
      <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-muted">
        <li>Create a container — a small server of its own, with its own address.</li>
        <li>Deploy an app into it from its repository.</li>
        <li>Give it a domain, and turn on https.</li>
      </ol>
      <button
        onClick={onNewContainer}
        className="mt-3 rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30"
      >
        New container
      </button>
    </section>
  )
}

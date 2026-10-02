import { href } from '../lib/useRoute.js'

// One headline, not four decorative counters. The counters that remain are
// filters: each opens the list it counts, already narrowed to it. A number you
// cannot act on is wallpaper.
export default function Verdict({ data, problemCount }) {
  const running = data.instances.filter((i) => i.status === 'running').length
  const stopped = data.instances.length - running
  const secured = data.routes.filter((r) => r.ssl).length

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

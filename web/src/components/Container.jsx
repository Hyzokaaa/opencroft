import { useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import LogView from './LogView.jsx'
import { useServices } from '../lib/useContainer.js'

// What a container actually is, once something has been deployed into it.
//
// The list of containers answers "what exists"; this answers "what is it
// doing, and is it working" — which is the question you have when something
// is wrong, and until now could only be asked over ssh.
//
// The order follows that question: what reaches it, what it runs, and how to
// undo it. Domains are identity and stay short; snapshots are a cellar and
// grow without limit, so they go last.
export default function Container({
  container,
  routes,
  commandMode,
  onDeploy,
  onRollback,
  onAddDomain,
}) {
  const { data, error, fetchedAt, read } = useServices(container.name)
  const [logs, setLogs] = useState(null)

  const services = data?.services ?? []
  const snapshots = data?.snapshots ?? []
  const domains = routes.filter((r) => r.target === container.address)

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <h1 className="truncate font-mono text-base">{container.name}</h1>
          <Chip
            label={container.status}
            tone={
              container.status === 'running'
                ? 'border-running/40 text-running'
                : 'border-edge-strong text-muted'
            }
          />
        </div>

        <button
          onClick={() => onDeploy(container)}
          className="rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30"
        >
          Deploy a project
        </button>
      </div>

      <p className="font-mono text-xs text-muted">
        {container.image} · {container.address}
        {container.cpuLimit ? ` · ${container.cpuLimit} CPU` : ''}
        {container.memLimit ? ` · ${container.memLimit}` : ''}
      </p>

      {error && (
        <div className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
          <span className="text-caution">Could not read what is inside.</span>{' '}
          <span className="text-muted">{error}</span>
        </div>
      )}

      <Card
        title="Domains"
        count={domains.length}
        commandMode={commandMode}
        commands={['nginx -T | grep server_name']}
        action={
          <button
            onClick={() => onAddDomain(container)}
            className="text-xs text-muted transition hover:text-ink"
          >
            Add
          </button>
        }
      >
        {domains.length === 0 ? (
          <p className="px-4 py-5 text-center text-xs text-muted">Nothing points here yet.</p>
        ) : (
          <ul className="divide-y divide-edge">
            {domains.map((route) => (
              <li key={route.domain} className="flex items-center justify-between px-4 py-2.5">
                <span className="font-mono text-xs">
                  <span className="text-muted">{route.ssl ? 'https://' : 'http://'}</span>
                  {route.domain}
                </span>
                <span className="font-mono text-[11px] text-muted">:{route.port}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card
        title="Services"
        count={read ? services.length : undefined}
        commandMode={commandMode}
        commands={[`lxc exec ${container.name} -- systemctl list-units 'croft-*'`]}
      >
        {!read ? (
          <Reading what="what is running inside" />
        ) : services.length === 0 ? (
          <Empty onDeploy={() => onDeploy(container)} />
        ) : (
          <>
            <ul className="divide-y divide-edge">
              {services.map((service) => (
                <Service
                  key={service.name}
                  service={service}
                  onLogs={() => setLogs(service.name)}
                  onDeploy={() => onDeploy(container, service)}
                />
              ))}
            </ul>

            {/* Repeated on every row, this stops being read by the second one.
                Said once, about all of them, it says something the repetitions
                did not: that rolling the container back has nowhere safe to
                land. */}
            {services.length > 1 && services.every((s) => !s.healthy) && (
              <p className="border-t border-edge px-4 py-2.5 text-[11px] text-caution">
                None of these has a version recorded as having worked, so a rollback here cannot
                promise anything yet.
              </p>
            )}

            {fetchedAt && (
              <p className="border-t border-edge px-4 py-2 text-[11px] text-muted">
                Read from the machine at {fetchedAt.toLocaleTimeString()}, and again every five
                seconds.
              </p>
            )}
          </>
        )}
      </Card>

      <Snapshots
        snapshots={snapshots}
        services={services}
        container={container}
        commandMode={commandMode}
        read={read}
        onRollback={onRollback}
      />

      {logs && <LogView container={container.name} service={logs} onClose={() => setLogs(null)} />}
    </>
  )
}

// A skeleton of invented rows would suggest we know how many there are. We do
// not know anything yet, and saying so is the whole point.
function Reading({ what }) {
  return (
    <p role="status" className="px-4 py-8 text-center text-xs text-muted">
      Reading {what}&hellip;
    </p>
  )
}

function Empty({ onDeploy }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm text-muted">Nothing is deployed here.</p>
      <p className="mt-1 text-xs text-muted">
        A container is a small server. Put a project on it and it becomes something.
      </p>
      <button
        onClick={onDeploy}
        className="mt-3 rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30"
      >
        Deploy a project
      </button>
    </div>
  )
}

const PRIMARY =
  'rounded border border-edge-strong bg-raised px-2 py-0.5 text-xs transition hover:border-ink/30'
const SECONDARY =
  'rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink'

// State comes from the machine, not from what we recorded. A service croft
// deployed and that then died must look dead here — and in more than one
// colour, because colour alone reaches nobody who cannot see it.
function Service({ service, onLogs, onDeploy }) {
  const running = service.state === 'active'

  return (
    <li className={`relative px-4 py-3 ${running ? '' : 'bg-problem/[0.04]'}`}>
      {!running && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StateDot state={service.state} />
            <span className="font-mono text-sm font-medium">{service.name}</span>
            {service.runtime && <Chip label={service.runtime} tone="border-edge text-muted" />}

            {!running && service.state && (
              <span className="rounded border border-problem/40 px-1.5 py-px text-[11px] text-problem">
                {service.state}
              </span>
            )}

            {/* The fact belongs on the row; the reason belongs one click away,
                where every other explanation on this panel already lives. */}
            {!service.healthy && (
              <Chip
                label="no known-good version"
                tone="border-caution/40 text-caution"
                explain="Nothing is recorded as having worked for this service, so there is no point to go back to. Set a readiness check and deploy again to get one."
              />
            )}
          </div>

          <p className="mt-1 truncate pl-[18px] font-mono text-[11px] text-muted">
            {service.repo}
            {service.branch ? ` @ ${service.branch}` : ''}
            {service.commit ? ` · ${service.commit.slice(0, 7)}` : ''}
          </p>

          <p className="pl-[18px] font-mono text-[11px] text-muted">
            {service.path}
            {service.port ? ` · :${service.port}` : ''}
            {service.health?.path ? ` · ready on ${service.health.path}` : ''}
          </p>
        </div>

        {/* When something is down the useful move is to look before writing,
            so the prominent button follows the situation rather than the
            layout. */}
        <div className="flex shrink-0 gap-1.5">
          <button onClick={onLogs} className={running ? SECONDARY : PRIMARY}>
            Logs
          </button>
          <button onClick={onDeploy} className={SECONDARY}>
            Deploy again
          </button>
        </div>
      </div>
    </li>
  )
}

// Three shapes, not three colours: filled, hollow, ringed. Plus the word
// itself for anyone reading with their ears — a title attribute reaches
// neither a screen reader reliably nor a touch screen at all.
function StateDot({ state }) {
  const label = state === 'active' ? 'running' : state || 'unknown'

  const shape =
    state === 'active'
      ? 'bg-running'
      : !state || state === 'unknown'
        ? 'border border-stopped'
        : 'bg-problem ring-2 ring-problem/25'

  return (
    <span className={`size-2.5 shrink-0 rounded-full ${shape}`}>
      <span className="sr-only">{label}</span>
    </span>
  )
}

// croft-deploy-<service>-<YYYYMMDD>-<HHMMSS>. An earlier generation left the
// service out; that is not guessed at, it is said.
function describe(name) {
  const match = /^croft-deploy-(?:(.+)-)?(\d{8})-(\d{6})$/.exec(name)
  if (!match) {
    return { raw: name, ours: name.startsWith('croft-'), service: null, when: null }
  }

  const [, service, day, time] = match
  const when = new Date(
    `${day.slice(0, 4)}-${day.slice(4, 6)}-${day.slice(6, 8)}T` +
      `${time.slice(0, 2)}:${time.slice(2, 4)}:${time.slice(4, 6)}Z`,
  )

  return {
    raw: name,
    ours: true,
    service: service ?? null,
    when: isNaN(when.getTime()) ? null : when,
  }
}

function ago(when) {
  if (!when) return null

  const minutes = Math.round((Date.now() - when.getTime()) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  if (minutes < 60 * 24) return `${Math.round(minutes / 60)}h ago`
  return `${Math.round(minutes / 1440)}d ago`
}

// Ours and yours in one list, told apart by the prefix — the same rule the
// generated vhosts use. Yours are shown because a rollback has to name
// something real, and never touched because they are not ours to touch.
function Snapshots({ snapshots, services, container, commandMode, read, onRollback }) {
  const healthy = new Set(services.map((s) => s.healthy).filter(Boolean))
  const alive = new Set(services.map((s) => s.name))

  // Sorted by what can be read out of the name, rather than by trusting the
  // order the daemon happened to return. An assumption like that breaks
  // silently, and the panel would lie about which one is newest.
  const rows = snapshots
    .map(describe)
    .sort((a, b) => (b.when?.getTime() ?? 0) - (a.when?.getTime() ?? 0))

  const worked = rows.filter((s) => healthy.has(s.raw))
  const rest = rows.filter((s) => !healthy.has(s.raw))

  const row = (snapshot) => (
    <Snapshot
      key={snapshot.raw}
      snapshot={snapshot}
      healthy={healthy.has(snapshot.raw)}
      orphaned={Boolean(snapshot.service) && !alive.has(snapshot.service)}
      onRollback={() => onRollback(container, snapshot.raw)}
    />
  )

  return (
    <Card
      title="Snapshots"
      count={read ? snapshots.length : undefined}
      commandMode={commandMode}
      commands={[`lxc info ${container.name}`]}
    >
      {!read ? (
        <Reading what="what there is to go back to" />
      ) : snapshots.length === 0 ? (
        <p className="px-4 py-6 text-center text-xs text-muted">
          None yet. One is taken before every deployment.
        </p>
      ) : (
        <>
          <ul className="divide-y divide-edge">
            {worked.map(row)}
            {rest.slice(0, 4).map(row)}
          </ul>

          {/* Folded, never hidden — and the count says exactly how much. */}
          {rest.length > 4 && (
            <details className="border-t border-edge">
              <summary className="cursor-pointer px-4 py-2 text-xs text-muted transition hover:text-ink">
                {rest.length - 4} older
              </summary>
              <ul className="divide-y divide-edge">{rest.slice(4).map(row)}</ul>
            </details>
          )}
        </>
      )}

      <p className="border-t border-edge px-4 py-2.5 text-[11px] text-muted">
        Snapshots are of the whole container, so restoring one reaches everything in it — and
        everything written since.
      </p>
    </Card>
  )
}

function Snapshot({ snapshot, healthy, orphaned, onRollback }) {
  const when = ago(snapshot.when)

  return (
    <li className="group flex items-center justify-between gap-3 px-4 py-2">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-xs">
            {snapshot.service ?? (snapshot.ours ? 'an earlier version of croft' : 'taken by hand')}
            {when && <span className="text-muted"> · {when}</span>}
          </span>

          {!snapshot.ours && (
            <Chip
              label="yours"
              tone="border-yours/40 text-yours"
              explain="Croft did not take this, and will never remove it."
            />
          )}
          {healthy && (
            <Chip
              label="worked"
              tone="border-running/40 text-running"
              explain="This version passed its readiness check. It is where a rollback goes back to, and it is never pruned."
            />
          )}
          {orphaned && (
            <Chip
              label="service is gone"
              tone="border-edge-strong text-muted"
              explain="The service that took this snapshot is no longer on the container, so nothing prunes it. Restoring it would bring that service back, along with everything else from that moment."
            />
          )}
        </div>

        {/* The raw name stays: it is what you type if you end up doing this by
            hand, which is the whole point of the tool. */}
        <p className="truncate font-mono text-[11px] text-muted">{snapshot.raw}</p>
      </div>

      <button
        onClick={onRollback}
        className={`shrink-0 rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-problem/50 hover:text-problem group-hover:opacity-100 focus:opacity-100 ${
          healthy ? '' : 'opacity-0'
        }`}
      >
        Restore&hellip;
      </button>
    </li>
  )
}

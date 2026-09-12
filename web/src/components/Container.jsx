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
export default function Container({ container, routes, onBack, onDeploy, onRollback, onAddDomain }) {
  const { data, error } = useServices(container.name)
  const [logs, setLogs] = useState(null)

  const services = data?.services ?? []
  const snapshots = data?.snapshots ?? []
  const domains = routes.filter((r) => r.target === container.address)

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <button
            onClick={onBack}
            className="rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
          >
            ← Containers
          </button>
          <h1 className="truncate font-mono text-base">{container.name}</h1>
          <Chip
            label={container.status}
            tone={container.status === 'running' ? 'border-running/40 text-running' : 'border-edge-strong text-muted'}
          />
        </div>

        <button
          onClick={() => onDeploy(container)}
          className="rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30"
        >
          Deploy a project
        </button>
      </div>

      <p className="text-xs text-faint">
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
        title="Services"
        count={services.length}
        commands={[`lxc exec ${container.name} -- systemctl list-units 'croft-*'`]}
      >
        {services.length === 0 ? (
          <Empty onDeploy={() => onDeploy(container)} />
        ) : (
          <ul className="divide-y divide-edge/50">
            {services.map((service) => (
              <Service
                key={service.name}
                service={service}
                container={container}
                onLogs={() => setLogs(service.name)}
                onDeploy={() => onDeploy(container, service)}
              />
            ))}
          </ul>
        )}
      </Card>

      <div className="grid gap-4 xl:grid-cols-[55fr_45fr]">
        <Snapshots
          snapshots={snapshots}
          services={services}
          container={container}
          onRollback={onRollback}
        />

        <Card
          title="Domains"
          count={domains.length}
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
            <p className="px-4 py-6 text-center text-xs text-muted">
              Nothing points here yet.
            </p>
          ) : (
            <ul className="divide-y divide-edge/50">
              {domains.map((route) => (
                <li key={route.domain} className="flex items-center justify-between px-4 py-2.5">
                  <span className="font-mono text-xs">
                    {route.ssl && <span className="text-faint">🔒 </span>}
                    {route.domain}
                  </span>
                  <span className="font-mono text-[11px] text-faint">:{route.port}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {logs && <LogView container={container.name} service={logs} onClose={() => setLogs(null)} />}
    </>
  )
}

function Empty({ onDeploy }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm text-muted">Nothing is deployed here.</p>
      <p className="mt-1 text-xs text-faint">
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

// State comes from the machine, not from what we recorded. A service croft
// deployed and that then died must look dead here.
function Service({ service, onLogs, onDeploy }) {
  const running = service.state === 'active'

  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <StateDot state={service.state} />
            <span className="font-mono text-sm font-medium">{service.name}</span>
            {service.runtime && <Chip label={service.runtime} tone="border-edge text-faint" />}
            {!running && service.state && (
              <span className="text-xs text-problem">{service.state}</span>
            )}
          </div>

          <p className="mt-1 truncate pl-[18px] font-mono text-[11px] text-faint">
            {service.repo}
            {service.branch ? ` @ ${service.branch}` : ''}
            {service.commit ? ` · ${service.commit.slice(0, 7)}` : ''}
          </p>

          <p className="pl-[18px] font-mono text-[11px] text-faint">
            {service.path}
            {service.port ? ` · :${service.port}` : ''}
            {service.health?.path ? ` · ready on ${service.health.path}` : ''}
          </p>

          {/* Without a version recorded as working there is nothing to go back
              to, and finding that out during an incident is too late. */}
          {!service.healthy && (
            <p className="mt-1 pl-[18px] text-[11px] text-caution">
              No version of this is recorded as having worked — set a readiness check and deploy
              again to get one.
            </p>
          )}
        </div>

        <div className="flex shrink-0 gap-1.5">
          <button
            onClick={onLogs}
            className="rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
          >
            Logs
          </button>
          <button
            onClick={onDeploy}
            className="rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
          >
            Deploy again
          </button>
        </div>
      </div>
    </li>
  )
}

function StateDot({ state }) {
  if (state === 'active') {
    return <span className="size-2.5 shrink-0 rounded-full bg-running" title="running" />
  }
  if (!state || state === 'unknown') {
    return <span className="size-2.5 shrink-0 rounded-full border border-stopped" title="unknown" />
  }
  return (
    <span className="size-2.5 shrink-0 rounded-full bg-problem ring-2 ring-problem/25" title={state} />
  )
}

// Ours and yours in one list, told apart by the prefix — the same rule the
// generated vhosts use. Yours are shown because a rollback has to name
// something real, and never touched because they are not ours to touch.
function Snapshots({ snapshots, services, container, onRollback }) {
  const healthy = new Set(services.map((s) => s.healthy).filter(Boolean))

  return (
    <Card
      title="Snapshots"
      count={snapshots.length}
      commands={[`lxc info ${container.name}`]}
    >
      {snapshots.length === 0 ? (
        <p className="px-4 py-6 text-center text-xs text-muted">
          None yet. One is taken before every deployment.
        </p>
      ) : (
        <ul className="divide-y divide-edge/50">
          {[...snapshots].reverse().map((name) => (
            <li key={name} className="flex items-center justify-between gap-3 px-4 py-2">
              <div className="min-w-0">
                <span className="truncate font-mono text-[11px]">{name}</span>
                <div className="mt-0.5 flex gap-1.5">
                  {!name.startsWith('croft-') && (
                    <Chip label="yours" tone="border-yours/40 text-yours" explain="Croft did not take this, and will never remove it." />
                  )}
                  {healthy.has(name) && (
                    <Chip label="worked" tone="border-running/40 text-running" explain="This version passed its readiness check." />
                  )}
                </div>
              </div>

              <button
                onClick={() => onRollback(container, name)}
                className="shrink-0 rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-problem/50 hover:text-problem"
              >
                Restore
              </button>
            </li>
          ))}
        </ul>
      )}

      <p className="px-4 py-2.5 text-[11px] text-faint">
        Snapshots are of the whole container, so restoring one reaches everything in it — and
        everything written since.
      </p>
    </Card>
  )
}

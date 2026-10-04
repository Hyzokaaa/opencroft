import { useEffect, useId, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Gated from './Gated.jsx'
import LogView from './LogView.jsx'
import PlanDialog from './PlanDialog.jsx'
import Actions from './Actions.jsx'
import { routeActions } from './RouteTable.jsx'
import { useServices, useDatabases } from '../lib/useContainer.js'
import { href } from '../lib/useRoute.js'
import { SEVERITY, VERBS, remediesFor } from '../lib/vocabulary.js'
import { readJSON } from '../lib/api.js'

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
  onDestroy,
  onPowerService,
  onRedeploy,
  onEnvironment,
  onPowerUnit,
  onAdopt,
  onAddDomain,
  onMoveProject,
  onPower,
  onDestroyContainer,
  routeHandlers,
  findings = [],
  instances = [],
  onRemedy,
  runtime = 'lxc',
}) {
  const { data, error, unavailable, fetchedAt, read, reload } = useServices(container.name)
  const databases = useDatabases(container.name)
  const [offersRead, setOffersRead] = useState(0)
  const offers = useShareable(container.name, `${container.project}-${offersRead}`)
  const [logs, setLogs] = useState(null)
  const [dialog, setDialog] = useState(null)

  const services = data?.services ?? []
  const external = data?.external ?? []
  const sites = data?.sites ?? []
  const snapshots = data?.snapshots ?? []
  // What reaches this container: whole domains, and prefixes of domains that
  // send one path here while the rest goes elsewhere.
  // Each keeps its route, so the same actions the Domains list offers are
  // offered here: this is where a person lands when something is wrong.
  const domains = routes.flatMap((r) => [
    ...(r.target === container.address ? [{ key: r.domain, domain: r.domain, ssl: r.ssl, port: r.port, route: r }] : []),
    ...(r.paths ?? [])
      .filter((p) => p.target === container.address)
      .map((p) => ({ key: r.domain + p.prefix, domain: r.domain + p.prefix, ssl: r.ssl, port: p.port, route: r, prefix: p.prefix })),
  ])
  const running = container.status === 'running'
  // What the home page says is wrong with this container, said here too.
  const mine = new Set([container.name, ...domains.filter((d) => !d.prefix).map((d) => d.domain)])
  const problems = findings.filter((f) => f.severity !== 'info' && mine.has(f.subject))
  // Why deploying is not possible right now, when it is not: said once, beside
  // the buttons it stops, rather than discovered as an error after the form.
  const blockedId = useId()
  const deployBlocked = !running ? null : unavailable ? 'This host cannot deploy services.' : null

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

        {/* A stopped container can be started and nothing else here: what
            comes next is starting it, so that is the button that stands out,
            and the two that need it running say so. */}
        {running ? (
          <div className="flex flex-wrap items-center justify-end gap-2">
            <button onClick={() => onPower(container, true)} className={HEADER_SECONDARY}>
              Stop
            </button>
            <button onClick={() => onAddDomain(container)} className={HEADER_SECONDARY}>
              Add domain
            </button>
            <Gated reason={deployBlocked} onClick={() => onDeploy(container)} className={HEADER_PRIMARY}>
              Deploy a service
            </Gated>
          </div>
        ) : (
          <div className="flex flex-wrap items-center justify-end gap-2">
            <span id={blockedId} className="text-xs text-muted">
              Start it first: deploying and adding need it running.
            </span>
            <Gated reasonId={blockedId} className={HEADER_SECONDARY}>
              Add domain
            </Gated>
            <Gated reasonId={blockedId} className={HEADER_SECONDARY}>
              Deploy a service
            </Gated>
            <button onClick={() => onPower(container, false)} className={HEADER_PRIMARY}>
              Start
            </button>
          </div>
        )}
      </div>

      <p className="font-mono text-xs text-muted">
        {container.image} · {container.address}
        {container.internalName ? ` · ${container.internalName}` : ''}
        {container.cpuLimit ? ` · ${container.cpuLimit} CPU` : ''}
        {container.memLimit ? ` · ${container.memLimit}` : ''}
      </p>

      {/* The project is a label on the container: moving it changes nothing
          it runs, so it sits here with the facts rather than among actions. */}
      <p className="text-xs text-muted">
        {container.project ? (
          <>
            In the project{' '}
            <a href={href('projects', container.project)} className="font-mono text-ink underline-offset-4 hover:underline">
              {container.project}
            </a>
          </>
        ) : (
          'In no project'
        )}
        {' · '}
        <button onClick={() => onMoveProject(container)} className="text-muted underline-offset-2 transition hover:text-ink hover:underline">
          Move&hellip;
        </button>
      </p>

      {problems.map((f) => {
        const tone = SEVERITY[f.severity] ?? SEVERITY.warning
        const remedies = remediesFor(f, { instances, routes }).filter((r) => r.act !== 'open')
        return (
          <div key={f.kind + f.subject} className="flex flex-wrap items-center gap-3 rounded-lg border border-edge bg-panel px-4 py-2.5 text-xs">
            <span className={`h-4 w-[3px] rounded ${tone.accent}`} aria-hidden="true" />
            <span className="min-w-0 flex-1">
              <span className="sr-only">{tone.label}: </span>
              <span className="font-mono">{f.subject}</span> <span className="text-muted">&mdash; {f.message}</span>
            </span>
            {remedies.map((r) => (
              <button
                key={r.act}
                onClick={() => onRemedy(r)}
                className="rounded border border-edge-strong bg-raised px-2 py-0.5 text-xs transition hover:border-ink/30"
              >
                {r.label}
              </button>
            ))}
          </div>
        )
      })}

      {/* Before anything was read, a failure is said in each card, with a way
          to try again. After, the last reading stays and this says how old
          it is — the same rule the whole panel follows. */}
      {read && error && (
        <div role="status" className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
          <span className="text-caution">Lost contact while reading what is inside.</span>{' '}
          <span className="text-muted">
            {error} Showing what was read{fetchedAt ? ` at ${fetchedAt.toLocaleTimeString()}` : ''}.
          </span>
        </div>
      )}

      <Card
        title="Domains"
        count={domains.length}
        commandMode={commandMode}
        commands={['nginx -T | grep server_name']}
        action={
          <Gated
            reasonId={running ? undefined : blockedId}
            onClick={() => onAddDomain(container)}
            className="text-xs text-muted transition hover:text-ink"
          >
            Add
          </Gated>
        }
      >
        {domains.length === 0 ? (
          <p className="px-4 py-5 text-center text-xs text-muted">Nothing points here yet.</p>
        ) : (
          <ul className="divide-y divide-edge">
            {domains.map((d) => (
              <li key={d.key} className="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5">
                <span className="font-mono text-xs">
                  <span className="text-muted">{d.ssl ? 'https://' : 'http://'}</span>
                  {d.domain}
                  <span className="text-muted"> &rarr; :{d.port}</span>
                </span>
                {d.prefix ? (
                  d.route.state === 'managed' && (
                    <Actions
                      label={`Actions for ${d.domain}`}
                      actions={[{ label: 'Stop sending it here', onClick: () => routeHandlers.onRemovePath(d.route, d.prefix) }]}
                    />
                  )
                ) : (
                  <Actions label={`Actions for ${d.domain}`} actions={routeActions(d.route, routeHandlers)} />
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card
        title="Services"
        count={read ? services.length : undefined}
        commandMode={commandMode}
        commands={[`${runtime} exec ${container.name} -- systemctl list-units 'croft-*'`]}
      >
        {unavailable ? (
          <Unavailable what="what runs inside its containers" reason={unavailable} />
        ) : !read ? (
          error ? (
            <Failed what="what is running inside" error={error} onRetry={reload} stopped={!running} />
          ) : (
            <Reading what="what is running inside" />
          )
        ) : services.length === 0 && external.length === 0 && sites.length === 0 ? (
          <Empty onDeploy={() => onDeploy(container)} blockedId={running ? undefined : blockedId} />
        ) : (
          <>
            {services.length > 0 && (
              <ul className="divide-y divide-edge">
                {services.map((service) => (
                  <Service
                    key={service.name}
                    service={service}
                    onLogs={() => setLogs({ name: service.name, kind: 'service' })}
                    onDeploy={() => onDeploy(container, service)}
                    onRedeploy={() => onRedeploy(container, service)}
                    onEnvironment={() => onEnvironment(container, service)}
                    onDestroy={() => onDestroy(container, service)}
                    onPower={(action) => onPowerService(container, service, action)}
                  />
                ))}
              </ul>
            )}

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

            {(external.length > 0 || sites.length > 0) && (
              <div className="border-t border-edge">
                <p className="px-4 pt-3 pb-1 text-[11px] text-faint">
                  Found on this container, not created by croft. You can restart or stop what is
                  already there — or take it on, so croft can redeploy it from its repository.
                </p>
                <ul className="divide-y divide-edge">
                  {external.map((unit) => (
                    <ExternalUnit
                      key={unit.name}
                      unit={unit}
                      onLogs={() => setLogs({ name: unit.name, kind: 'unit' })}
                      onPower={(action) => onPowerUnit(container, unit, action)}
                      onAdopt={() => onAdopt(container, { kind: 'units', name: unit.name })}
                    />
                  ))}
                  {sites.map((site) => (
                    <FoundSite
                      key={site.root}
                      site={site}
                      onAdopt={() => onAdopt(container, { kind: 'sites', name: site.domains[0] })}
                    />
                  ))}
                </ul>
              </div>
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

      <Databases
        container={container}
        state={databases}
        runtime={runtime}
        blockedId={running ? undefined : blockedId}
        commandMode={commandMode}
        onAdd={() => setDialog({ kind: 'add' })}
        onRemove={(database) => setDialog({ kind: 'remove', database })}
        offers={offers}
        onConnect={(offer) => setDialog({ kind: 'connect', offer })}
      />

      <Snapshots
        snapshots={snapshots}
        services={services}
        container={container}
        commandMode={commandMode}
        read={read}
        error={error}
        unavailable={unavailable}
        onRetry={reload}
        runtime={runtime}
        onRollback={onRollback}
        databases={databases.data?.databases ?? []}
      />

      <Card title="Danger zone">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <p className="text-xs text-muted">
            Destroying {container.name} deletes it with its disk, its snapshots and every database inside it.
          </p>
          <button
            onClick={() => onDestroyContainer(container.name)}
            className="rounded border border-problem/50 px-2.5 py-1 text-xs text-problem transition hover:bg-problem/10"
          >
            {VERBS.destroy} container&hellip;
          </button>
        </div>
      </Card>

      {logs && (
        <LogView
          container={container.name}
          service={logs.name}
          kind={logs.kind}
          onClose={() => setLogs(null)}
        />
      )}

      {dialog && (
        <PlanDialog
          request={databaseRequest(container, dialog)}
          onClose={() => setDialog(null)}
          onFinished={() => {
            setDialog(null)
            databases.reload()
            setOffersRead((n) => n + 1)
          }}
        />
      )}
    </>
  )
}

// databaseRequest is the whole add-and-remove flow, because PlanDialog already
// is one: a form, then the commands, then watching them run. A second dialog
// that did the same thing differently is how two screens start disagreeing.
function databaseRequest(container, dialog) {
  const url = `/api/hosts/local/instances/${container.name}/databases`

  if (dialog.kind === 'remove') {
    // One that lives elsewhere is let go of: the data stays where it is.
    const away = Boolean(dialog.database.location)
    return {
      title: away
        ? `Disconnect ${container.name} from ${dialog.database.name} in ${dialog.database.location}`
        : `Remove ${dialog.database.name} from ${container.name}`,
      url: `${url}/${dialog.database.name}`,
      method: 'DELETE',
      destructive: !away,
      // Dropping the data is the one thing here a snapshot alone brings back.
      ...(away ? {} : { confirm: dialog.database.name, verb: `Drop ${dialog.database.name}` }),
    }
  }

  if (dialog.kind === 'connect') {
    return {
      title: `Connect ${container.name} to ${dialog.offer.name} in ${dialog.offer.container}`,
      url: `${url}/connect`,
      method: 'POST',
      defaults: { location: dialog.offer.container, name: dialog.offer.name },
    }
  }

  return {
    title: `Add a database to ${container.name}`,
    url,
    method: 'POST',
    fields: [
      {
        name: 'engine',
        label: 'Engine',
        options: ['postgres', 'mysql', 'redis'],
      },
      {
        name: 'name',
        label: 'Name',
        placeholder: 'main',
        autoFocus: true,
        hint: 'Lowercase letters, digits and underscores — it becomes an SQL identifier.',
      },
    ],
    defaults: { engine: 'postgres', name: 'main' },
  }
}

// The data lives in the container, so it is in the snapshots too. That is the
// advantage, and it is also the thing to know before restoring one.
// The unit each engine runs as inside the container, which is what is asked
// when checking it by hand — the same names the agent asks about.
const ENGINE_UNIT = { postgres: 'postgresql', mysql: 'mariadb', redis: 'redis-server' }

function Databases({ container, state, runtime, blockedId, commandMode, onAdd, onRemove, offers, onConnect }) {
  const databases = state.data?.databases ?? []
  // The command for what is actually here, not for postgres whatever it is.
  const engines = [...new Set(databases.filter((d) => !d.location).map((d) => d.engine))]
  const units = engines.length ? engines.map((e) => ENGINE_UNIT[e] ?? e) : Object.values(ENGINE_UNIT)

  return (
    <Card
      title="Databases"
      count={state.read ? databases.length : undefined}
      commandMode={commandMode}
      commands={[`${runtime} exec ${container.name} -- systemctl is-active ${units.join(' ')}`]}
      action={
        <Gated
          reason={state.unavailable ? 'This host cannot add databases.' : undefined}
          reasonId={state.unavailable ? undefined : blockedId}
          onClick={onAdd}
          className="text-xs text-muted transition hover:text-ink"
        >
          Add
        </Gated>
      }
    >
      {state.unavailable ? (
        <Unavailable what="the databases inside its containers" reason={state.unavailable} />
      ) : !state.read ? (
        state.error ? (
          <Failed what="what it stores" error={state.error} onRetry={state.reload} />
        ) : (
          <Reading what="what it stores" />
        )
      ) : databases.length === 0 ? (
        <div className="px-4 py-8 text-center">
          <p className="text-sm text-muted">No database here.</p>
          <p className="mt-1 text-xs text-muted">
            One inside this container is captured by its snapshots — the application and its data
            go back together.
          </p>
        </div>
      ) : (
        <ul className="divide-y divide-edge">
          {databases.map((database) => (
            <Database key={database.name} database={database} onRemove={() => onRemove(database)} />
          ))}
        </ul>
      )}

      {state.read && state.error && (
        <p role="status" className="border-t border-edge px-4 py-2 text-[11px] text-caution">
          Could not read them again just now: {state.error} This is the last reading.
        </p>
      )}

      {/* What the other containers of its project hold, offered rather than
          hidden: a backend beside its database is the default, and a second
          service reaching the same data is the usual next step. */}
      {offers.length > 0 && (
        <div className="border-t border-edge px-4 py-3">
          <p className="mb-2 text-xs text-muted">In this project, and could connect from here:</p>
          <ul className="space-y-1.5">
            {offers.map((offer) => (
              <li key={offer.container + '/' + offer.name} className="flex items-center justify-between gap-3">
                <span className="font-mono text-xs">
                  {offer.name}
                  <span className="text-muted"> · {offer.engine} in {offer.container}</span>
                </span>
                <button onClick={() => onConnect(offer)} className={SECONDARY}>
                  Connect&hellip;
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  )
}

function Database({ database, onRemove }) {
  const away = Boolean(database.location)
  const running = database.state === 'active'
  const wrong = !away && !running

  return (
    <li className={`relative px-4 py-3 ${wrong ? 'bg-problem/[0.04]' : ''}`}>
      {wrong && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-xs">
            {database.name}
            <span className="text-muted"> · {database.engine}</span>
          </p>
          <p className="mt-0.5 font-mono text-[11px] text-muted">
            {database.db} on {database.port}
            {away ? ` · in ${database.location}` : ''}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Chip
            label={away ? 'elsewhere' : database.state || 'unknown'}
            tone={
              away
                ? 'border-caution/40 text-caution'
                : running
                  ? 'border-running/40 text-running'
                  : 'border-problem/40 text-problem'
            }
          />
          <button onClick={onRemove} className={SECONDARY}>
            {away ? 'Disconnect' : 'Remove'}
          </button>
        </div>
      </div>

      {away && (
        <p className="mt-2 text-[11px] text-caution">
          This data is in {database.location}, so restoring a snapshot of this container does not
          bring it back — the application would return to an older version against today&apos;s
          data.
        </p>
      )}
    </li>
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

// Something failed before anything was read. Saying "Reading…" for ever was
// the old answer; this one says what failed and offers the obvious next move.
function Failed({ what, error, onRetry, stopped }) {
  return (
    <div role="alert" className="px-4 py-8 text-center">
      <p className="text-sm text-problem">Could not read {what}.</p>
      <p className="mt-1 text-xs text-muted">
        {stopped ? 'The container is stopped — start it, and what is inside can be read. ' : ''}
        {error}
      </p>
      <button onClick={onRetry} className={`mt-3 ${PRIMARY}`}>
        Try again
      </button>
    </div>
  )
}

// A host whose croft runs without its privileged side cannot look inside a
// container at all. That is how the host is set up, not a failure: no retry,
// and a word on why — so it is not mistaken for something broken here.
function Unavailable({ what, reason }) {
  return (
    <div className="px-4 py-8 text-center">
      <p className="text-sm text-muted">This host cannot read {what}.</p>
      <p className="mt-1 text-xs text-muted">
        Croft runs here without its privileged side, which is the part that works inside
        containers. The daemon said: {reason}.
      </p>
    </div>
  )
}

function Empty({ onDeploy, blockedId }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm text-muted">Nothing is deployed here.</p>
      <p className="mt-1 text-xs text-muted">
        A container is a small server. Deploy an app from its repository and it becomes something.
      </p>
      <Gated
        reasonId={blockedId}
        onClick={onDeploy}
        className="mt-3 rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30"
      >
        Deploy a service
      </Gated>
    </div>
  )
}

const HEADER_PRIMARY =
  'rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30'
const HEADER_SECONDARY =
  'rounded border border-edge px-2.5 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink'
const PRIMARY =
  'rounded border border-edge-strong bg-raised px-2 py-0.5 text-xs transition hover:border-ink/30'
const SECONDARY =
  'rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink'

// State comes from the machine, not from what we recorded. A service croft
// deployed and that then died must look dead here — and in more than one
// colour, because colour alone reaches nobody who cannot see it.
function Service({ service, onLogs, onDeploy, onRedeploy, onEnvironment, onDestroy, onPower }) {
  const running = service.state === 'active'
  const site = service.adopted?.site
  // An adopted unit that reads no environment file has none to edit.
  const environment = !service.adopted || site || service.adopted.envFile

  // Every verb the row has, in the order they are reached for. A site is
  // files the web server reads: there is no process of its own to read a
  // journal of, restart or stop. Croft did not put an adopted service there,
  // so it only lets go of it — and the button says which of the two it does.
  const all = [
    !site && { key: 'logs', label: 'Logs', onClick: onLogs },
    { key: 'redeploy', label: 'Redeploy', onClick: onRedeploy },
    !site && running && { key: 'restart', label: 'Restart', onClick: () => onPower('restart') },
    !site && running && { key: 'stop', label: 'Stop', onClick: () => onPower('stop') },
    !site && !running && { key: 'start', label: 'Start', onClick: () => onPower('start') },
    environment && { key: 'environment', label: 'Environment…', onClick: onEnvironment },
    { key: 'properties', label: 'Properties…', onClick: onDeploy },
    service.adopted
      ? { key: 'release', label: 'Release…', onClick: onDestroy }
      : { key: 'remove', label: 'Remove…', onClick: onDestroy, danger: true },
  ].filter(Boolean)
  const visible = !site && !running ? ['logs', 'start'] : service.pending ? ['redeploy'] : site ? [] : ['logs']
  const next = all
    .filter((a) => visible.includes(a.key))
    .map((a) => ({ ...a, primary: (a.key === 'logs' && !running) || (a.key === 'redeploy' && service.pending) }))
  const rest = all.filter((a) => !visible.includes(a.key))

  return (
    <li className={`relative px-4 py-3 ${running ? '' : 'bg-problem/[0.04]'}`}>
      {!running && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StateDot state={service.state} />
            <span className="font-mono text-sm font-medium">{service.name}</span>
            {service.runtime && <Chip label={service.runtime} tone="border-edge text-muted" />}
            {site && <Chip label="site" tone="border-edge text-muted" />}
            {service.adopted && (
              <Chip
                label="adopted"
                tone="border-yours/40 text-yours"
                explain={
                  site
                    ? `Found being served and taken on. Croft fetches, builds and publishes it to ${site}; the web server's configuration${
                        service.adopted.envFile ? ` and ${service.adopted.envFile}` : ''
                      } stay exactly as whoever wrote them.`
                    : `Found running and taken on. Croft fetches, builds and restarts it; the unit ${service.adopted.unit}${
                        service.adopted.envFile ? ` and ${service.adopted.envFile}` : ''
                      } stay exactly as whoever wrote them.`
                }
              />
            )}

            {service.pending && (
              <Chip
                label="changes not deployed"
                tone="border-yours/40 text-yours"
                explain="Saved in Properties, not running yet. Redeploy to apply them."
              />
            )}

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
            {site ? ` → ${site}` : service.adopted ? ` · ${service.adopted.unit}` : ''}
            {service.port ? ` · :${service.port}` : ''}
            {service.health?.path ? ` · ready on ${service.health.path}` : ''}
          </p>
        </div>

        {/* The move the situation calls for stands on its own; the rest wait
            in the row's actions, removing last. When something is down the
            useful move is to look before writing — then to start it. With
            saved changes waiting, it is to deploy them. */}
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
          {next.map((a) => (
            <button key={a.key} onClick={a.onClick} className={a.primary ? PRIMARY : SECONDARY}>
              {a.label}
            </button>
          ))}
          <Actions label={`More for ${service.name}`} actions={rest} />
        </div>
      </div>
    </li>
  )
}

// A unit croft found rather than deployed. No repo, no branch, no health
// check — it never inspected the code to know any of that — so it gets the
// verbs that need none of it: look, and bounce the process.
// A directory the container's own web server serves, found in its
// configuration. It has no process of its own, so the only thing to offer is
// taking it on — which is what makes updating it possible from here.
function FoundSite({ site, onAdopt }) {
  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="size-2.5 shrink-0 rounded-sm border border-stopped" aria-hidden />
            <span className="font-mono text-sm">{site.domains.join(', ')}</span>
            <Chip label="site" tone="border-edge text-muted" />
            <Chip
              label="found here"
              tone="border-yours/40 text-yours"
              explain="Found in the web server's configuration inside this container. Croft did not publish it, so it cannot rebuild it until it is taken on."
            />
          </div>
          <p className="mt-1 pl-[18px] font-mono text-[11px] text-muted">served from {site.root}</p>
        </div>
        <button onClick={onAdopt} className={SECONDARY}>
          Adopt&hellip;
        </button>
      </div>
    </li>
  )
}

function ExternalUnit({ unit, onLogs, onPower, onAdopt }) {
  const running = unit.state === 'active'

  return (
    <li className={`relative px-4 py-3 ${running ? '' : 'bg-problem/[0.04]'}`}>
      {!running && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <StateDot state={unit.state} />
          <span className="font-mono text-sm">{unit.name}</span>
          <Chip
            label="found here"
            tone="border-yours/40 text-yours"
            explain="Found on this container. Croft did not deploy it, so it has no snapshots and no redeploy — only what any process gets: logs, restart, stop and start."
          />
          {!running && unit.state && (
            <span className="rounded border border-problem/40 px-1.5 py-px text-[11px] text-problem">
              {unit.state}
            </span>
          )}
        </div>

        <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
          <button onClick={onLogs} className={running ? SECONDARY : PRIMARY}>
            Logs
          </button>
          {running ? (
            <>
              <button onClick={() => onPower('restart')} className={SECONDARY}>
                Restart
              </button>
              <button onClick={() => onPower('stop')} className={SECONDARY}>
                Stop
              </button>
            </>
          ) : (
            <button onClick={() => onPower('start')} className={PRIMARY}>
              Start
            </button>
          )}
          <button onClick={onAdopt} className={SECONDARY}>
            Adopt&hellip;
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

// croft-<kind>-<service>-<YYYYMMDD>-<HHMMSS>. An earlier generation left the
// service out; that is not guessed at, it is said.
function describe(name) {
  const match = /^croft-(deploy|destroy)-(?:(.+)-)?(\d{8})-(\d{6})$/.exec(name)
  if (!match) {
    return { raw: name, ours: name.startsWith('croft-'), service: null, when: null }
  }

  const [, kind, service, day, time] = match
  const when = new Date(
    `${day.slice(0, 4)}-${day.slice(4, 6)}-${day.slice(6, 8)}T` +
      `${time.slice(0, 2)}:${time.slice(2, 4)}:${time.slice(4, 6)}Z`,
  )

  return {
    raw: name,
    ours: true,
    kind,
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
// What restoring one of these actually reaches, once the container stores data.
//
// A snapshot that carries the database is the whole reason for putting it in
// here — and it is also the reason a restore is not free. Saying only the first
// half is how the first half stops being believed.
function DataWarning({ databases }) {
  const inside = databases.filter((d) => !d.location).map((d) => d.db)
  const away = databases.filter((d) => d.location)

  if (inside.length === 0 && away.length === 0) return null

  return (
    <div className="border-b border-edge px-4 py-2.5 text-[11px]">
      {inside.length > 0 && (
        <p className="text-caution">
          Restoring takes {inside.join(', ')} back to that moment too, losing anything written
          since.
        </p>
      )}
      {away.map((d) => (
        <p key={d.name} className="text-caution">
          {d.db} is in {d.location} and does <strong>not</strong> go back — the application would
          return to an older version against today&apos;s data.
        </p>
      ))}
    </div>
  )
}

function Snapshots({ snapshots, services, container, commandMode, read, error, unavailable, onRetry, runtime, onRollback, databases }) {
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
      commands={[`${runtime} info ${container.name}`]}
    >
      {unavailable ? (
        <Unavailable what="the snapshots of its containers" reason={unavailable} />
      ) : !read ? (
        error ? (
          <Failed what="what there is to go back to" error={error} onRetry={onRetry} />
        ) : (
          <Reading what="what there is to go back to" />
        )
      ) : snapshots.length === 0 ? (
        <p className="px-4 py-6 text-center text-xs text-muted">
          None yet. One is taken before every deployment.
        </p>
      ) : (
        <>
          {/* The same capability sold as the advantage and warned about as the
              risk. Said here, above the buttons, because this is where somebody
              is about to press one. */}
          <DataWarning databases={databases ?? []} />

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
          {snapshot.kind === 'destroy' && (
            <Chip
              label="removed here"
              tone="border-caution/40 text-caution"
              explain="Taken immediately before this service was removed. Nothing ever prunes it, because it is the only way back to something you chose to delete."
            />
          )}
          {/* Green says "running" here and nothing else; having worked once
              is a fact about the past, so it is a tick on a neutral chip. */}
          {healthy && (
            <Chip
              label="✓ worked"
              tone="border-edge-strong text-ink"
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
        className="shrink-0 rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-problem/50 hover:text-problem"
      >
        Restore&hellip;
      </button>
    </li>
  )
}

// useShareable asks which databases of this container's project it could
// connect to. Read when the container or its project changes, and after a
// connection is made or let go — not on a timer: it changes only when
// somebody does one of those.
function useShareable(container, key) {
  const [offers, setOffers] = useState([])
  useEffect(() => {
    let current = true
    // An offer that cannot be read is no offer: nothing here depends on it,
    // so a failure leaves the list empty rather than in the way.
    fetch(`/api/hosts/local/instances/${container}/databases/shareable`)
      .then(readJSON)
      .then((payload) => current && setOffers(payload ?? []))
      .catch(() => current && setOffers([]))
    return () => {
      current = false
    }
  }, [container, key])
  return offers
}

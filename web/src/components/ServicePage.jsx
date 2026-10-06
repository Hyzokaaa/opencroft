import { useId, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Gated from './Gated.jsx'
import LogView from './LogView.jsx'
import PlanDialog from './PlanDialog.jsx'
import DomainList from './DomainList.jsx'
import {
  Problems, Snapshots, StateDot, Reading, Failed, Unavailable, snapshotRemoval,
  HEADER_PRIMARY, HEADER_SECONDARY, SECONDARY,
} from './Container.jsx'
import { useServices, useDatabases } from '../lib/useContainer.js'
import { href } from '../lib/useRoute.js'
import { VERBS } from '../lib/vocabulary.js'
import { portOf, reaching } from '../lib/domains.js'

// One service: what it is doing, what reaches it, and everything done to it.
//
// A container's page answers "what runs here"; this answers the question
// asked during an incident — "is the support desk up, and if not, why" — and
// puts the one move the answer calls for at the top. Domains come first
// because they are how anybody else meets the service, and they are given to
// it here: which vhost a name ends up in is croft's business, not the
// person's.
export default function ServicePage({
  container,
  name,
  routes,
  certificates = [],
  findings = [],
  instances = [],
  publicAddress,
  commandMode,
  runtime = 'lxc',
  onDeploy,
  onRedeploy,
  onPowerService,
  onEnvironment,
  onDestroy,
  onRollback,
  onAddDomain,
  onPower,
  routeHandlers,
  onRemedy,
}) {
  const { data, error, unavailable, fetchedAt, read, reload } = useServices(container.name)
  const databases = useDatabases(container.name)
  const [logs, setLogs] = useState(false)
  const [deleting, setDeleting] = useState(null)
  const stoppedId = useId()

  const services = data?.services ?? []
  const service = services.find((s) => s.name === name)

  // Before anything is known about it there is nothing to say about it: the
  // page says what it is waiting for, or what failed, and nothing else.
  if (unavailable) {
    return (
      <>
        <Card title={name}><Unavailable what="what runs inside its containers" reason={unavailable} /></Card>
      </>
    )
  }
  if (!read) {
    return (
      <>
        <Card title={name}>
          {error ? (
            <Failed what={name} error={error} onRetry={reload} stopped={container.status !== 'running'} />
          ) : (
            <Reading what={name} />
          )}
        </Card>
      </>
    )
  }
  if (!service) {
    return (
      <>
        <div className="rounded-lg border border-edge bg-panel px-5 py-6 text-sm">
          <p>
            There is no service called <span className="font-mono">{name}</span> in{' '}
            <span className="font-mono">{container.name}</span>. It may have been removed meanwhile.
          </p>
          <a href={href('containers', container.name)} className="mt-2 inline-block text-xs text-muted underline underline-offset-4 hover:text-ink">
            What runs in {container.name}
          </a>
        </div>
      </>
    )
  }

  const up = container.status === 'running'
  const running = service.state === 'active'
  const site = service.adopted?.site
  const port = portOf(service, container)
  const names = reaching(routes, container.address, port)
  const whole = names.filter((n) => !n.prefix)
  // What the home page says is wrong with it, said here too: anything about
  // a name that reaches it, or about the container it lives in.
  const mine = new Set([container.name, ...whole.map((n) => n.domain)])
  const problems = findings.filter((f) => f.severity !== 'info' && mine.has(f.subject))
  // An adopted unit that reads no environment file has none to edit.
  const environment = !service.adopted || site || service.adopted.envFile
  const noPort = port ? null : `${name} listens on no port croft knows of, so a domain has nowhere to lead.`
  const addDomain = () => onAddDomain(container, service)

  // The one move the situation calls for, and the sentence that says why.
  // A stopped container stops everything in it, so starting it comes first;
  // a service that is down is looked at before it is written to; saved
  // changes wait for a redeploy; and running, reached by nobody, is half
  // done.
  const situation = !up
    ? { sentence: `${container.name} is stopped, so ${name} is not running.`, primary: 'start-container' }
    : !running && !site
      ? { sentence: `${name} is ${service.state || 'not running'}${whole.length ? `, so ${whole[0].domain} answers with an error page` : ''}.`, primary: 'logs' }
      : service.pending
        ? { sentence: `${name} is running. Changes saved in Properties are not deployed yet.`, primary: 'redeploy' }
        : whole.length === 0 && names.length > 0
          ? { sentence: `${name} is running, reached only at ${names.map((n) => n.domain).join(', ')}.`, primary: null }
        : whole.length === 0
          ? { sentence: noPort ? `${name} is running. It listens on no port, so no domain can reach it.` : `${name} is running on :${port}, and no domain reaches it yet.`, primary: noPort ? null : 'add-domain' }
          : {
              sentence: `${name} is running and answers at ${whole[0].domain}${
                whole.length > 1 ? ` and ${whole.length - 1} other domain${whole.length > 2 ? 's' : ''}` : ''
              }.`,
              primary: null,
            }

  const verbs = [
    !site && { key: 'logs', label: 'Logs', onClick: () => setLogs(true) },
    !site && running && { key: 'restart', label: 'Restart', onClick: () => onPowerService(container, service, 'restart') },
    !site && running && { key: 'stop', label: 'Stop', onClick: () => onPowerService(container, service, 'stop') },
    !site && !running && { key: 'start', label: 'Start', onClick: () => onPowerService(container, service, 'start') },
    { key: 'redeploy', label: 'Redeploy', onClick: () => onRedeploy(container, service) },
  ].filter(Boolean)

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2.5">
          <StateDot state={up ? service.state : 'inactive'} />
          <h1 className="truncate font-mono text-base">{name}</h1>
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
          {up && !running && service.state && (
            <span className="rounded border border-problem/40 px-1.5 py-px text-[11px] text-problem">
              {service.state}
            </span>
          )}
        </div>

        {/* The move the situation calls for stands out; the others are there
            and quiet. With the container stopped, none of them can work, and
            the one reason is said once beside them. */}
        <div className="flex flex-wrap items-center justify-end gap-2">
          {!up && (
            <span id={stoppedId} className="text-xs text-muted">
              Start {container.name} first.
            </span>
          )}
          {verbs.map((v) => (
            <Gated
              key={v.key}
              reasonId={up ? undefined : stoppedId}
              onClick={v.onClick}
              className={situation.primary === v.key ? HEADER_PRIMARY : HEADER_SECONDARY}
            >
              {v.label}
            </Gated>
          ))}
          {situation.primary === 'add-domain' && (
            <button onClick={addDomain} className={HEADER_PRIMARY}>
              Add domain
            </button>
          )}
          {situation.primary === 'start-container' && (
            <button onClick={() => onPower(container, false)} className={HEADER_PRIMARY}>
              Start {container.name}
            </button>
          )}
        </div>
      </div>

      {situation.sentence && <p className="text-sm">{situation.sentence}</p>}

      <p className="font-mono text-xs text-muted">
        <span className="font-sans">in </span>
        <a href={href('containers', container.name)} className="text-ink underline-offset-4 hover:underline">
          {container.name}
        </a>
        {port ? ` · ${container.address}:${port}` : ''}
        {service.runtime ? ` · ${service.runtime}` : ''}
      </p>

      <Problems problems={problems} instances={instances} routes={routes} onRemedy={onRemedy} />

      {/* A lost read keeps the last one on screen and says how old it is,
          as every page does. */}
      {error && (
        <div role="status" className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
          <span className="text-caution">Lost contact while reading {name}.</span>{' '}
          <span className="text-muted">
            {error} Showing what was read{fetchedAt ? ` at ${fetchedAt.toLocaleTimeString()}` : ''}.
          </span>
        </div>
      )}

      <Card
        title="Domains"
        count={names.length}
        commandMode={commandMode}
        commands={port ? [`nginx -T | grep -B3 'proxy_pass http://${container.address}:${port}'`] : undefined}
        action={
          names.length > 0 && (
            <Gated
              reasonId={up ? undefined : stoppedId}
              onClick={addDomain}
              className="text-xs text-muted transition hover:text-ink"
            >
              Add domain
            </Gated>
          )
        }
      >
        {names.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <p className="text-sm text-muted">No domain reaches {name} yet.</p>
            {noPort ? (
              <p className="mt-1 text-xs text-muted">{noPort}</p>
            ) : (
              <>
                <p className="mx-auto mt-1 max-w-md text-xs text-muted">
                  Point the domain&apos;s DNS at this server
                  {publicAddress ? <> (<span className="font-mono text-ink">{publicAddress}</span>)</> : ''} with an A
                  record, then add it here. Croft serves it over http, then over https once it sees the
                  name arrive.
                </p>
                <Gated
                  reasonId={up ? undefined : stoppedId}
                  onClick={addDomain}
                  className={`mt-3 ${situation.primary === 'add-domain' ? HEADER_SECONDARY : HEADER_PRIMARY}`}
                >
                  Add domain
                </Gated>
              </>
            )}
          </div>
        ) : (
          <DomainList names={names} certificates={certificates} handlers={routeHandlers} publicAddress={publicAddress} />
        )}
      </Card>

      <Deployment service={service} container={container} onDeploy={onDeploy} onRedeploy={onRedeploy} blockedId={up ? undefined : stoppedId} />

      <Card title="Environment">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <p className="text-xs text-muted">
            {!environment
              ? `${name} reads no environment file croft knows of, so there is nothing to change here.`
              : service.adopted?.envFile
                ? <>The variables {name} starts with, read from <span className="font-mono">{service.adopted.envFile}</span>. Changing them restarts it.</>
                : `The variables ${name} starts with. Changing them restarts it; secrets are never shown back.`}
          </p>
          {environment && (
            <button onClick={() => onEnvironment(container, service)} className={SECONDARY}>
              Environment&hellip;
            </button>
          )}
        </div>
      </Card>

      <Snapshots
        only={name}
        snapshots={data?.snapshots ?? []}
        services={services}
        container={container}
        commandMode={commandMode}
        read={read}
        error={error}
        unavailable={unavailable}
        onRetry={reload}
        runtime={runtime}
        onRollback={onRollback}
        onDelete={setDeleting}
        databases={databases.data?.databases ?? []}
      />

      <Card title="Danger zone">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <p className="text-xs text-muted">
            {service.adopted
              ? `Releasing ${name} forgets what croft wrote down about it. Nothing it runs or serves is touched.`
              : `Removing ${name} stops it and deletes its files and environment, after a snapshot.${
                  whole.length ? ` ${whole.length === 1 ? 'Its domain stops' : `Its ${whole.length} domains stop`} reaching anything.` : ''
                }`}
          </p>
          <button
            onClick={() => onDestroy(container, service)}
            className={
              service.adopted
                ? SECONDARY
                : 'rounded border border-problem/50 px-2.5 py-1 text-xs text-problem transition hover:bg-problem/10'
            }
          >
            {service.adopted ? `${VERBS.release}…` : `Remove ${name}…`}
          </button>
        </div>
      </Card>

      {logs && <LogView container={container.name} service={name} onClose={() => setLogs(false)} />}

      {deleting && (
        <PlanDialog
          request={snapshotRemoval(container, deleting)}
          onClose={() => {
            setDeleting(null)
            reload()
          }}
          onFinished={reload}
        />
      )}
    </>
  )
}

// Where the service comes from and how it is started: the facts Properties
// changes, said rather than hidden behind the button.
function Deployment({ service, container, onDeploy, onRedeploy, blockedId }) {
  const site = service.adopted?.site
  const rows = [
    ['Repository', `${service.repo || '—'}${service.branch ? ` @ ${service.branch}` : ''}`],
    ['Deployed', service.commit ? service.commit.slice(0, 7) : 'not recorded'],
    ['Path', `${service.path || '—'}${site ? ` → ${site}` : service.adopted?.unit ? ` · ${service.adopted.unit}` : ''}`],
    !site && ['Port', service.port ? `:${service.port}` : container.port ? `:${container.port}, the container's` : null],
    ['Readiness', service.health?.path ? `${service.health.path}${service.health.status ? ` answers ${service.health.status}` : ''}` : null],
    ['Known good', service.healthy ?? null],
  ].filter(Boolean)

  return (
    <Card
      title="Deployment"
      action={
        <div className="flex items-center gap-2">
          <button onClick={() => onDeploy(container, service)} className={SECONDARY}>
            Properties&hellip;
          </button>
          <Gated reasonId={blockedId} onClick={() => onRedeploy(container, service)} className={SECONDARY}>
            Redeploy
          </Gated>
        </div>
      }
    >
      <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 px-4 py-3 text-xs">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted">{label}</dt>
            <dd className="min-w-0 break-all font-mono">
              {value ?? <span className="font-sans text-faint">none</span>}
            </dd>
          </div>
        ))}
      </dl>
      {/* The fact belongs here; what to do about it, beside it. */}
      {!service.healthy && (
        <p className="border-t border-edge px-4 py-2.5 text-[11px] text-caution">
          Nothing is recorded as having worked for {service.name}, so a rollback has no point to go back to.
          Set a readiness check in Properties and deploy again to get one.
        </p>
      )}
    </Card>
  )
}

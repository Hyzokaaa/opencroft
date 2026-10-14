import { useEffect, useId, useRef, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Gated from './Gated.jsx'
import CopyButton from './CopyButton.jsx'
import PlanDialog from './PlanDialog.jsx'
import DomainList from './DomainList.jsx'
import Activity, { DEPLOYS, ago } from './Activity.jsx'
import AutoDeploy, { useAutoDeploy } from './AutoDeploy.jsx'
import { LogPane } from './LogView.jsx'
import { Discard } from './PlanDialog.jsx'
import {
  Field, BuildFields, RunFields, HealthFields, body, remembered, repoProblem, inspection, detected,
} from './DeployDialog.jsx'
import {
  Problems, Snapshots, Reading, Failed, Unavailable, snapshotRemoval,
  HEADER_PRIMARY, HEADER_SECONDARY, SECONDARY, RAISED,
} from './Container.jsx'
import { useServices, useDatabases } from '../lib/useContainer.js'
import { href, serviceHref } from '../lib/useRoute.js'
import { VERBS } from '../lib/vocabulary.js'
import { portOf, reaching, stateOf } from '../lib/domains.js'
import { readJSON } from '../lib/api.js'

// One service: what it is doing, what reaches it, and everything done to it.
//
// A container's page answers "what runs here"; this answers the question
// asked during an incident — "is the support desk up, and if not, why" — and
// puts the one move the answer calls for at the top.
//
// It was one long scroll — domains, a form the height of the screen, the
// environment, snapshots — every part at the same weight, and whatever was
// wanted was somewhere below. Now the header stays and says how it is; each
// part is a page of its own, with its own address, chosen from a list beside
// it: Overview, then what is set (configuration, domains, environment,
// automatic deployments), then what happened (deployments, logs), and the
// danger zone apart at the end.
const PARTS = [
  { id: 'overview', label: 'Overview' },
  { group: 'Settings' },
  { id: 'configuration', label: 'Configuration' },
  { id: 'domains', label: 'Domains' },
  { id: 'environment', label: 'Environment' },
  { id: 'auto-deploy', label: 'Automatic deployments' },
  { group: 'Observe' },
  { id: 'deployments', label: 'Deployments' },
  { id: 'logs', label: 'Logs', process: true },
  { group: '' },
  { id: 'danger', label: 'Danger zone' },
]

export default function ServicePage({
  container,
  name,
  part: asked,
  routes,
  certificates = [],
  findings = [],
  instances = [],
  publicAddress,
  commandMode,
  runtime = 'lxc',
  jobsReading,
  agentDown,
  onRedeploy,
  onWrite = (title, open) => open(),
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
  const autoDeploy = useAutoDeploy(container.name, name)
  const [deleting, setDeleting] = useState(null)
  const stoppedId = useId()

  const services = data?.services ?? []
  const service = services.find((s) => s.name === name)

  // The configuration is edited in place. draft is what has been typed and
  // not saved yet; with none, the form shows what the container has saved,
  // and follows it as it is read again. It is kept for the tab's life, so
  // going to another page and back does not throw it away.
  const draftKey = `croft:draft:${container.name}/${name}`
  const [draft, setDraftState] = useState(() => {
    try {
      return JSON.parse(sessionStorage.getItem(draftKey) ?? 'null')
    } catch {
      return null
    }
  })
  const setDraft = (next) => {
    setDraftState(next)
    try {
      if (next) sessionStorage.setItem(draftKey, JSON.stringify(next))
      else sessionStorage.removeItem(draftKey)
    } catch {
      // A browser that keeps nothing still keeps it on this page.
    }
  }
  const [why, setWhy] = useState('')
  const [writing, setWriting] = useState(null)
  const [settling, setSettling] = useState(null)
  const [notice, setNotice] = useState(null)
  const original = service ? remembered(service) : null
  const form = draft ?? original
  const changed = Boolean(draft && original) && JSON.stringify(body(draft)) !== JSON.stringify(body(original))

  // A write has finished and the service been read again. If what is saved
  // now is what was typed, the draft has done its job; if not — a deployment
  // that stopped saves nothing — the draft stays, and the page says so,
  // instead of the change vanishing with the dialog.
  useEffect(() => {
    if (!settling) return
    setSettling(null)
    if (!changed) {
      setDraft(null)
      setWhy('')
      setNotice(null)
    } else {
      setNotice(
        settling === 'deploy'
          ? 'The deployment stopped, so nothing was saved. Your changes are still in Configuration: fix what it stopped on and try again, or save them for later.'
          : 'What is saved is still not what Configuration says. Your changes are still there, not saved.',
      )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settling, changed])

  // A draft that matches what is saved — typed back, or saved from
  // elsewhere — is no draft: keeping it would show old values as new.
  useEffect(() => {
    if (draft && original && !changed && !settling) setDraft(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changed, Boolean(original)])

  // Leaving the panel with typed changes asks first, as closing a form does.
  useEffect(() => {
    if (!changed) return
    const warn = (e) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [changed])

  // Before anything is known about it there is nothing to say about it: the
  // page says what it is waiting for, or what failed, and nothing else.
  if (unavailable) {
    return <Card title={name}><Unavailable what="what runs inside its containers" reason={unavailable} /></Card>
  }
  if (!read) {
    return (
      <Card title={name}>
        {error ? (
          <Failed what={name} error={error} onRetry={reload} stopped={container.status !== 'running'} />
        ) : (
          <Reading what={name} />
        )}
      </Card>
    )
  }
  if (!service) {
    return (
      <div className="rounded-lg border border-edge bg-panel px-5 py-6 text-sm">
        <p>
          There is no service called <span className="font-mono">{name}</span> in{' '}
          <span className="font-mono">{container.name}</span>. It may have been removed meanwhile.
        </p>
        <a href={href('containers', container.name)} className="mt-2 inline-block text-xs text-muted underline underline-offset-4 hover:text-ink">
          What runs in {container.name}
        </a>
      </div>
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
  const to = (p) => serviceHref(container.name, name, p)

  const parts = PARTS.filter((p) => !(p.process && site))
  const part = parts.some((p) => p.id === asked) ? asked : 'overview'

  // What the configuration form can do, said once and used by the form's
  // own buttons and by the header's.
  const runnable = Boolean(service.adopted) || Boolean(form.start?.trim())
  const formProblem = repoProblem(form.repo)
    ? 'The repository address needs fixing before this can be saved.'
    : !site && (form.health ?? '').trim() && !Number(form.port)
      ? 'A readiness check is asked on a port: say which one it listens on.'
      : !runnable
        ? 'Nothing says how to start it, so there is nothing to deploy.'
        : null
  const edit = (next) => {
    setNotice(null)
    setDraft(next)
  }
  const save = () => onWrite(`Save the configuration of ${name}`, () => setWriting('save'))
  const saveAndDeploy = () => onWrite(`Save and redeploy ${name}`, () => setWriting('deploy'))
  const lookAgain = () => onWrite(`Look at ${form.repo}`, () => setWriting('inspect'))
  const finished = (what) => async () => {
    await reload()
    setSettling(what)
  }

  // The one move the situation calls for, and the sentence that says why.
  // A stopped container stops everything in it, so starting it comes first;
  // a service that is down is looked at before it is written to; typed and
  // saved changes wait for a redeploy; and otherwise, redeploying is what
  // this page is for.
  const situation = !up
    ? { sentence: `${container.name} is stopped, so ${name} is not running.`, primary: 'start-container' }
    : !running && !site
      ? { sentence: `${name} is ${service.state || 'not running'}${whole.length ? `, so ${whole[0].domain} answers with an error page` : ''}.`, primary: 'logs' }
      : changed
        ? { sentence: `${name} is running. Its configuration has changes that are not saved yet.`, primary: 'redeploy' }
      : service.pending
        ? { sentence: `${name} is running. Saved configuration changes are not deployed yet.`, primary: 'redeploy' }
        : whole.length === 0 && names.length > 0
          ? { sentence: `${name} is running, reached only at ${names.map((n) => n.domain).join(', ')}.`, primary: 'redeploy' }
        : whole.length === 0
          ? { sentence: noPort ? `${name} is running. It listens on no port, so no domain can reach it.` : `${name} is running on :${port}, and no domain reaches it yet.`, primary: noPort ? 'redeploy' : 'add-domain' }
          : {
              sentence: `${name} is running and answers at ${whole[0].domain}${
                whole.length > 1 ? ` and ${whole.length - 1} other domain${whole.length > 2 ? 's' : ''}` : ''
              }.`,
              primary: 'redeploy',
            }

  const verbs = [
    !site && running && { key: 'restart', label: 'Restart', onClick: () => onPowerService(container, service, 'restart') },
    !site && running && { key: 'stop', label: 'Stop', onClick: () => onPowerService(container, service, 'stop') },
    !site && !running && { key: 'start', label: 'Start', onClick: () => onPowerService(container, service, 'start') },
    // With the form changed, redeploying what is saved would quietly leave
    // out what was just typed: the one button does what the form says.
    changed
      ? { key: 'redeploy', label: 'Save and redeploy…', onClick: saveAndDeploy, reason: up ? formProblem : null }
      : { key: 'redeploy', label: 'Redeploy', onClick: () => onRedeploy(container, service) },
  ].filter(Boolean)

  const snapshotCount = (data?.snapshots ?? []).filter((s) => s.includes(`-${name}-`)).length
  const subject = `${container.name}/${name}`
  const badges = {
    configuration: changed ? { dot: true, sr: 'not saved' } : null,
    domains: { text: String(names.length) },
    'auto-deploy': autoDeploy.read ? { text: autoDeploy.data?.enabled ? 'on' : 'off' } : null,
    deployments: snapshotCount ? { text: String(snapshotCount), sr: `${snapshotCount} snapshots` } : null,
  }

  return (
    <>
      {/* Who, how it is, and what to do about it: the same on every part. */}
      <header className="space-y-1.5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2.5">
            <h1 className="truncate font-mono text-base">{name}</h1>
            <StatePill up={up} state={service.state} site={site} />
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
                explain="Saved in Configuration, not running yet. Redeploy to apply them."
              />
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
            {!site && part !== 'logs' && (
              <a href={to('logs')} className={situation.primary === 'logs' ? HEADER_PRIMARY : HEADER_SECONDARY}>
                Logs
              </a>
            )}
            {verbs.map((v) => (
              <Gated
                key={v.key}
                reason={v.reason}
                reasonId={up ? undefined : stoppedId}
                onClick={v.onClick}
                // On Configuration with changes, its own bar holds the same
                // button, beside the fields; one solid button per view.
                className={situation.primary === v.key && !(part === 'configuration' && changed) ? HEADER_PRIMARY : HEADER_SECONDARY}
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

        <p className="text-sm">
          {situation.sentence}
          {changed && part !== 'configuration' && (
            <>
              {' '}
              <a href={to('configuration')} className="text-yours underline underline-offset-4">Review the changes</a>
            </>
          )}
        </p>

        <p className="flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-muted">
          <span className="font-sans">in</span>
          <a href={href('containers', container.name)} className="text-ink underline-offset-4 hover:underline">
            {container.name}
          </a>
          {port ? <span>· {container.address}:{port}</span> : null}
          {service.runtime ? <span>· {service.runtime}</span> : null}
        </p>
      </header>

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

      {/* A failed deployment saves nothing; the draft stays, and that is said
          wherever the person is, since the button was in the header. */}
      {notice && (
        <p role="alert" className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs text-caution">
          {notice}
          {part !== 'configuration' && (
            <>
              {' '}
              <a href={to('configuration')} className="underline underline-offset-2">Open Configuration</a>
            </>
          )}
        </p>
      )}

      <div className="grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[12.5rem_minmax(0,1fr)] lg:items-start">
        <SubNav parts={parts} part={part} to={to} badges={badges} label={`${name}: sections`} />

        <div className="min-w-0 max-w-6xl space-y-4">
          {part === 'overview' && (
            <Overview
              service={service}
              container={container}
              port={port}
              names={names}
              certificates={certificates}
              noPort={noPort}
              autoDeploy={autoDeploy}
              jobs={jobsReading?.jobs ?? []}
              subject={subject}
              to={to}
              stoppedId={up ? undefined : stoppedId}
              onAddDomain={addDomain}
            />
          )}

          {part === 'configuration' && (
            <Configuration
              service={service}
              form={form}
              onChange={edit}
              changed={changed}
              why={why}
              problem={formProblem}
              repoWrong={repoProblem(form.repo)}
              stoppedId={up ? undefined : stoppedId}
              onLookAgain={lookAgain}
              onSave={save}
              onDeploy={saveAndDeploy}
              onDiscard={() => {
                setDraft(null)
                setWhy('')
                setNotice(null)
              }}
            />
          )}

          {part === 'domains' && (
            <Card
              title="Domains"
              count={names.length}
              description={`The names that reach ${name}${port ? ` on :${port}` : ''}. Each opens the site.`}
              commandMode={commandMode}
              commands={port ? [`nginx -T | grep -B3 'proxy_pass http://${container.address}:${port}'`] : undefined}
              action={
                names.length > 0 && (
                  <Gated reasonId={up ? undefined : stoppedId} onClick={addDomain} className={HEADER_SECONDARY}>
                    Add domain
                  </Gated>
                )
              }
            >
              {names.length === 0 ? (
                <NoDomain name={name} noPort={noPort} publicAddress={publicAddress} stoppedId={up ? undefined : stoppedId} onAddDomain={addDomain} />
              ) : (
                <DomainList names={names} certificates={certificates} handlers={routeHandlers} publicAddress={publicAddress} />
              )}
            </Card>
          )}

          {part === 'environment' && (
            <Environment container={container} service={service} environment={environment} onEdit={() => onEnvironment(container, service)} />
          )}

          {part === 'auto-deploy' && <AutoDeploy service={service} state={autoDeploy} agentDown={agentDown} />}

          {part === 'deployments' && (
            <>
              <Activity
                subject={subject}
                title={`What was done to ${name}`}
                reading={jobsReading}
                commandMode={commandMode}
                empty={`Nothing has been done to ${name} from this panel yet.`}
              />
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
            </>
          )}

          {part === 'logs' && (
            <section aria-label={`Logs of ${name}`} className="flex h-[70vh] min-h-80 flex-col overflow-hidden rounded-lg border border-edge bg-panel">
              <LogPane container={container.name} service={name} title="Logs" />
            </section>
          )}

          {part === 'danger' && (
            <Card title="Danger zone">
              <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                <p className="max-w-prose text-xs text-muted">
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
          )}
        </div>
      </div>

      {/* Each write shows its plan first. Closing one leaves the form as it
          was: the draft lives on the page, not in the dialog. */}
      {writing === 'inspect' && (
        <PlanDialog
          request={inspection(container, { repo: form.repo.trim(), branch: form.branch, name, path: form.path })}
          onClose={() => setWriting(null)}
          onBack={() => setWriting(null)}
          onResult={(detection) => {
            setWhy(
              detection.found === false
                ? 'nothing recognisable in it, so the commands below are the ones it already had'
                : (detection.why || `found ${detection.runtime || 'something'}`).replace(/\.$/, ''),
            )
            edit(detected({ repo: form.repo.trim(), branch: form.branch, name, path: form.path }, detection, form))
            setWriting(null)
          }}
        />
      )}
      {writing === 'save' && (
        <PlanDialog
          request={{
            title: `Save the configuration of ${name}`,
            url: `/api/hosts/local/instances/${container.name}/services/${name}/properties`,
            method: 'PUT',
            defaults: body(form),
            verb: 'Save',
          }}
          onClose={() => setWriting(null)}
          onBack={() => setWriting(null)}
          onFinished={finished('save')}
        />
      )}
      {writing === 'deploy' && (
        <PlanDialog
          request={{
            title: `Save and redeploy ${name}`,
            url: `/api/hosts/local/instances/${container.name}/services/deploy`,
            method: 'POST',
            defaults: body(form),
            verb: 'Redeploy',
          }}
          onClose={() => setWriting(null)}
          onBack={() => setWriting(null)}
          onFinished={finished('deploy')}
        />
      )}

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

// How it is, in a word and not only a colour: running is the one green.
function StatePill({ up, state, site }) {
  if (!up) return <Chip label="container stopped" tone="border-edge-strong text-muted" />
  if (site) return null
  if (state === 'active') return <Chip label="running" tone="border-running/40 text-running" />
  return <Chip label={state || 'unknown'} tone="border-problem/40 text-problem" />
}

// The parts of the page: a column beside it on a wide screen, a row of tabs
// above it on a narrow one — scrolled sideways inside itself, never the page.
function SubNav({ parts, part, to, badges, label }) {
  // On a phone the row is wider than the screen: the part being shown is
  // brought into view, rather than left past the edge.
  const list = useRef(null)
  useEffect(() => {
    const ul = list.current
    const here = ul?.querySelector('[aria-current="page"]')
    if (!ul || !here || ul.scrollWidth <= ul.clientWidth) return
    ul.scrollLeft = here.parentElement.offsetLeft - ul.offsetLeft - 16
  }, [part])

  return (
    <nav aria-label={label} className="min-w-0 lg:sticky lg:top-16">
      <ul ref={list} className="-mx-4 flex gap-1 overflow-x-auto border-b border-edge px-4 pb-2 md:-mx-6 md:px-6 lg:mx-0 lg:block lg:space-y-px lg:border-0 lg:p-0">
        {parts.map((p, i) =>
          p.group !== undefined ? (
            <li key={`g${i}`} aria-hidden="true" className="hidden lg:block">
              {p.group ? (
                <span className="block px-2.5 pb-1 pt-4 text-[11px] uppercase tracking-wide text-faint">{p.group}</span>
              ) : (
                <span className="my-3 block border-t border-edge" />
              )}
            </li>
          ) : (
            <li key={p.id} className="shrink-0">
              <a
                href={to(p.id === 'overview' ? null : p.id)}
                aria-current={p.id === part ? 'page' : undefined}
                className={`flex items-center justify-between gap-2 whitespace-nowrap rounded px-2.5 py-1.5 text-sm transition ${
                  p.id === part ? 'bg-raised text-ink' : 'text-muted hover:bg-white/[0.03] hover:text-ink'
                } ${p.id === 'danger' && p.id !== part ? 'hover:text-problem' : ''}`}
              >
                <span>{p.label}</span>
                <Badge badge={badges[p.id]} />
              </a>
            </li>
          ),
        )}
      </ul>
    </nav>
  )
}

function Badge({ badge }) {
  if (!badge) return null
  if (badge.dot) {
    return (
      <>
        <span aria-hidden="true" className="size-1.5 rounded-full bg-yours" />
        <span className="sr-only"> — {badge.sr}</span>
      </>
    )
  }
  return (
    <span className="text-xs text-faint">
      <span aria-hidden={badge.sr ? true : undefined}>{badge.text}</span>
      {badge.sr && <span className="sr-only">{badge.sr}</span>}
    </span>
  )
}

// What it is and where it stands, on one screen: the facts somebody asks for
// first, each one copyable; how it is reached; what was done to it last.
function Overview({ service, container, port, names, certificates, noPort, autoDeploy, jobs, subject, to, stoppedId, onAddDomain }) {
  const site = service.adopted?.site
  const last = jobs.find((j) => j.subject === subject && DEPLOYS.has(j.kind))
  const auto = autoDeploy.data?.enabled

  const facts = [
    ['Repository', service.repo, { copy: true, wide: true }],
    ['Branch', service.branch],
    ['Deployed commit', service.commit ? service.commit.slice(0, 7) : null, { copy: service.commit }],
    ['Internal address', port ? `${container.address}:${port}` : container.address, { copy: true }],
    ['Runtime', service.runtime || null],
    ['Where it lands', service.path, { copy: true }],
    site ? ['Published to', site, { copy: true }] : service.adopted?.unit ? ['Runs as', service.adopted.unit] : null,
    !site && ['Readiness check', service.health?.path || null, { none: 'none — ends when the unit starts' }],
    ['Known good', service.healthy ?? null, { none: 'nothing recorded yet' }],
  ].filter(Boolean)

  return (
    <>
      <Card
        title="Details"
        action={<a href={to('configuration')} className="text-xs text-muted transition hover:text-ink">Edit configuration →</a>}
      >
        <dl className="grid gap-x-6 gap-y-3 px-4 py-4 sm:grid-cols-2 xl:grid-cols-3">
          {facts.map(([label, value, opts = {}]) => (
            <Fact key={label} label={label} value={value} {...opts} />
          ))}
          <div className="min-w-0">
            <dt className="text-[11px] text-muted">Automatic deployments</dt>
            <dd className="mt-0.5 text-xs">
              <a href={to('auto-deploy')} className="underline-offset-4 hover:underline">
                {!autoDeploy.read ? '…' : auto ? `On — follows ${service.branch || 'its branch'}` : 'Off'}
              </a>
            </dd>
          </div>
        </dl>
        {/* The fact belongs here; what to do about it, beside it. */}
        {!service.healthy && !site && (
          <p className="border-t border-edge px-4 py-2.5 text-xs text-caution">
            Nothing is recorded as having worked, so a rollback has no point to go back to.{' '}
            <a href={to('configuration')} className="underline underline-offset-2">Set a readiness check</a> and deploy
            again to get one.
          </p>
        )}
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card
          title="Reached at"
          count={names.length}
          action={<a href={to('domains')} className="text-xs text-muted transition hover:text-ink">Manage domains →</a>}
        >
          {names.length === 0 ? (
            <div className="px-4 py-5 text-xs text-muted">
              {noPort ?? (
                <>
                  No domain reaches it yet.{' '}
                  <Gated reasonId={stoppedId} onClick={onAddDomain} className="text-ink underline underline-offset-4">
                    Add a domain
                  </Gated>
                </>
              )}
            </div>
          ) : (
            <ul className="divide-y divide-edge">
              {names.map((n) => {
                const state = stateOf(n, certificates)
                return (
                  <li key={n.key} className="flex flex-wrap items-center justify-between gap-x-3 gap-y-0.5 px-4 py-2">
                    <a
                      href={`${n.ssl ? 'https' : 'http'}://${n.domain}`}
                      target="_blank"
                      rel="noreferrer"
                      className="min-w-0 break-all font-mono text-xs underline-offset-4 hover:underline"
                    >
                      <span className="text-muted">{n.ssl ? 'https://' : 'http://'}</span>
                      {n.domain}
                      <span className="sr-only"> (opens in a new tab)</span>
                    </a>
                    <span className={`text-[11px] ${state.tone}`}>{state.label}</span>
                  </li>
                )
              })}
            </ul>
          )}
        </Card>

        <Card
          title="Last deployment"
          action={<a href={to('deployments')} className="text-xs text-muted transition hover:text-ink">All deployments →</a>}
        >
          {!last ? (
            <p className="px-4 py-5 text-xs text-muted">
              None from this panel yet{service.commit ? <> — <span className="font-mono">{service.commit.slice(0, 7)}</span> is what runs</> : ''}.
            </p>
          ) : (
            <div className="space-y-1 px-4 py-3 text-xs">
              <p>
                <span className={last.status === 'failed' ? 'text-problem' : last.status === 'running' ? 'text-caution' : ''}>
                  {last.status === 'done' ? '✓ worked' : last.status}
                </span>
                <span className="text-muted">
                  {' '}· {last.kind === 'auto-deploy' ? 'automatic, after a new commit' : 'started from the panel'} ·{' '}
                  {ago(new Date(last.started))}
                </span>
              </p>
              {last.status === 'failed' && last.error && (
                <p className="text-problem">
                  Stopped at step {last.reached} of {last.steps}: {last.error}
                </p>
              )}
            </div>
          )}
        </Card>
      </div>
    </>
  )
}

// One fact: a label, its value in full on hover, and a copy button when the
// value is something pasted elsewhere.
function Fact({ label, value, copy, none = 'none', wide }) {
  const text = typeof copy === 'string' ? copy : value
  return (
    <div className={`min-w-0 ${wide ? 'sm:col-span-2 xl:col-span-3' : ''}`}>
      <dt className="text-[11px] text-muted">{label}</dt>
      <dd className="mt-0.5 flex min-w-0 items-center gap-1.5">
        {value ? (
          <>
            <span className="min-w-0 truncate font-mono text-xs" title={String(value)}>{value}</span>
            {copy && <CopyButton text={String(text)} label={`Copy the ${label.toLowerCase()}`} className="shrink-0" />}
          </>
        ) : (
          <span className="text-xs text-faint">{none}</span>
        )}
      </dd>
    </div>
  )
}

function NoDomain({ name, noPort, publicAddress, stoppedId, onAddDomain }) {
  return (
    <div className="px-4 py-8 text-center">
      <p className="text-sm text-muted">No domain reaches {name} yet.</p>
      {noPort ? (
        <p className="mt-1 text-xs text-muted">{noPort}</p>
      ) : (
        <>
          <p className="mx-auto mt-1 max-w-md text-xs text-muted">
            Point the domain&apos;s DNS at this server
            {publicAddress ? <> (<span className="font-mono text-ink">{publicAddress}</span>)</> : ''} with an A
            record, then add it here. Croft serves it over http, then over https once it sees the name arrive.
          </p>
          <Gated reasonId={stoppedId} onClick={onAddDomain} className={`mt-3 ${RAISED}`}>
            Add domain
          </Gated>
        </>
      )}
    </div>
  )
}

// The variables it starts with: their names, read from the file, never their
// values — and the file's own editor, which shows its plan before writing.
function Environment({ container, service, environment, onEdit }) {
  const [file, setFile] = useState(null)
  const [failed, setFailed] = useState(null)
  const name = service.name

  useEffect(() => {
    if (!environment) return
    let current = true
    fetch(`/api/hosts/local/instances/${container.name}/services/${name}/env`)
      .then(readJSON)
      .then((payload) => current && setFile(payload))
      .catch((e) => current && setFailed(e.message))
    return () => {
      current = false
    }
  }, [container.name, name, environment])

  const keys = (file?.vars ?? []).map((v) => v.key)

  return (
    <Card
      title="Environment"
      count={file ? keys.length : undefined}
      description={
        !environment
          ? undefined
          : file?.applies === 'rebuild'
            ? 'Read by the build. Changing them rebuilds and publishes the site.'
            : `Read by ${name} when it starts. Changing them restarts it.`
      }
      action={
        environment && (
          <button onClick={onEdit} className={RAISED}>
            Edit environment&hellip;
          </button>
        )
      }
    >
      <div className="space-y-3 px-4 py-4 text-xs">
        {!environment ? (
          <p className="text-muted">{name} reads no environment file croft knows of, so there is nothing to change here.</p>
        ) : failed ? (
          <p className="text-muted">Could not read the file just now ({failed}). It can still be opened and edited.</p>
        ) : !file ? (
          <p role="status" className="text-muted">Reading the file&hellip;</p>
        ) : (
          <>
            <p className="flex flex-wrap items-center gap-1.5 text-muted">
              <span>In</span>
              <span className="font-mono text-ink">{file.file}</span>
              <CopyButton text={file.file} label="Copy the file's path" />
              {!file.exists && <span>— not there yet; editing creates it.</span>}
            </p>
            {keys.length === 0 ? (
              <p className="text-muted">No variables yet.</p>
            ) : (
              <>
                <ul aria-label="Variable names" className="flex flex-wrap gap-1.5">
                  {keys.map((k) => (
                    <li key={k} className="rounded border border-edge bg-ground px-1.5 py-0.5 font-mono text-[11px]">
                      {k}
                    </li>
                  ))}
                </ul>
                <p className="text-faint">Only the names are shown here. Values are in the editor.</p>
              </>
            )}
          </>
        )}
      </div>
    </Card>
  )
}

// Where the service comes from and how it is built, started and checked, as
// cards on the page rather than one form the height of the screen. Saving
// writes it for the next deployment; saving and redeploying puts it to work
// now. Reading the repository again only suggests.
//
// The bar with the buttons stays in sight while there are changes, so the
// way to save them is never a scroll away.
function Configuration({
  service, form, onChange, changed, why, problem, repoWrong, stoppedId,
  onLookAgain, onSave, onDeploy, onDiscard,
}) {
  const [left, setLeft] = useState(false)
  const [asking, setAsking] = useState(false)
  const statusId = useId()
  const site = service.adopted?.site
  const set = (key) => (v) => onChange({ ...form, [key]: v })
  const sourceMoved = form.repo !== (service.repo ?? '') || form.branch !== (service.branch ?? '')

  // Why the form's buttons cannot be used yet, in words, beside them.
  const status = problem
    ? problem
    : changed
      ? 'Not saved. Saving changes nothing that runs until the next deployment.'
      : service.pending
        ? 'Saved changes are waiting to be deployed — Redeploy, at the top, applies them.'
        : 'Nothing changed. Edit any field to change how it is deployed.'
  const blocked = !changed || Boolean(problem)

  return (
    <section aria-label="Configuration" className="space-y-4">
      <Card
        title="Source"
        description="Where the code comes from."
        action={
          <button
            onClick={() => {
              setLeft(true)
              if (!repoWrong) onLookAgain()
            }}
            className={HEADER_SECONDARY}
          >
            Read the repository again&hellip;
          </button>
        }
      >
        <div className="space-y-3 px-4 py-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
            <Field
              className="md:col-span-2 xl:col-span-1"
              label="Repository"
              hint="Public https. Private repositories need a deploy key, not built yet."
              invalid={left ? repoWrong : null}
              onBlur={() => setLeft(true)}
              placeholder="https://github.com/you/app.git"
              value={form.repo}
              onChange={set('repo')}
              mono
            />
            <Field label="Branch" value={form.branch} onChange={set('branch')} mono />
            <Field label="Where it lands" value={form.path} onChange={set('path')} mono hint="Directory in the container." />
          </div>
          {sourceMoved && !why && (
            <p className="text-xs text-muted">
              The commands below are still the ones saved for {service.repo || 'the old repository'}. Read the
              repository again to have them suggested from the new code, or change them yourself.
            </p>
          )}
        </div>
      </Card>

      <Card title="Build" description="Run in the checkout at every deployment, before it starts.">
        <div className="space-y-3 px-4 py-4">
          {why && (
            <p className="rounded border border-edge px-3 py-2 text-xs text-muted">
              Suggested from the repository: {why}. Check them, then save.
            </p>
          )}
          <BuildFields service={form} onChange={onChange} />
        </div>
      </Card>

      <Card title="Run" description={site ? 'How it is published.' : 'What keeps running, and where it listens.'}>
        <div className="px-4 py-4">
          <RunFields service={form} onChange={onChange} adopted={service.adopted} />
        </div>
      </Card>

      {!site && (
        <Card
          title="Readiness check"
          description="How croft knows a deployment worked — and so which version a rollback goes back to."
        >
          <div className="space-y-3 px-4 py-4">
            <HealthFields service={form} onChange={onChange} adopted={service.adopted} />
            {!service.healthy && (
              <p className="text-xs text-caution">
                Nothing is recorded as having worked for {service.name} yet. With a check here, the next deployment that
                passes it becomes the version a rollback returns to.
              </p>
            )}
          </div>
        </Card>
      )}

      <p className="text-xs text-faint">
        The environment is its own file, changed under Environment — saving this does not touch it.
      </p>

      <footer
        className={`${changed ? 'sticky bottom-0 z-10 border-yours/40 shadow-[0_-8px_24px_rgba(0,0,0,0.5)]' : 'border-edge'} flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-panel px-4 py-3`}
      >
        <p id={statusId} className={`text-xs ${changed && !problem ? 'text-yours' : service.pending && !changed ? 'text-yours' : 'text-muted'}`}>
          {status}
        </p>
        {asking ? (
          <Discard
            onKeep={() => setAsking(false)}
            onDiscard={() => {
              setAsking(false)
              setLeft(false)
              onDiscard()
            }}
          />
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            {changed && (
              <button
                onClick={() => setAsking(true)}
                className="px-1 text-xs text-muted underline decoration-dotted underline-offset-2 transition hover:text-ink"
              >
                Discard changes
              </button>
            )}
            <Gated reasonId={blocked ? statusId : undefined} onClick={onSave} className={HEADER_SECONDARY}>
              Save&hellip;
            </Gated>
            <Gated reasonId={blocked ? statusId : stoppedId} onClick={onDeploy} className={changed ? HEADER_PRIMARY : HEADER_SECONDARY}>
              Save and redeploy&hellip;
            </Gated>
          </div>
        )}
      </footer>
    </section>
  )
}

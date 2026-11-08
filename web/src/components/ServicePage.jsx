import { useEffect, useId, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Gated from './Gated.jsx'
import Actions from './Actions.jsx'
import CopyButton from './CopyButton.jsx'
import PlanDialog from './PlanDialog.jsx'
import DomainList from './DomainList.jsx'
import Activity, { DEPLOYS, ago, outcomeOf } from './Activity.jsx'
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
import { portOf, reaching, servedWith, stateOf } from '../lib/domains.js'
import { servingOf, toneOf } from '../lib/serving.js'
import { readJSON } from '../lib/api.js'
import { Status, Tabs } from './Listing.jsx'

// One service: what it is doing, what reaches it, and everything done to it.
//
// A container's page answers "what runs here"; this answers the question
// asked during an incident — "is the support desk up, and if not, why" — and
// puts the one move the answer calls for at the top.
//
// It was one long scroll — domains, a form the height of the screen, the
// environment, snapshots — every part at the same weight. Then a column of
// sections beside it, which on a laptop took a fifth of the width to say six
// words. Now the header stays and says how it is, and the parts are tabs
// under it, each with its own address. What decides how deployments happen —
// automatically or not, with a snapshot or not — sits with the deployments.
const PARTS = [
  { id: 'overview', label: 'Overview' },
  { id: 'configuration', label: 'Configuration' },
  { id: 'domains', label: 'Domains' },
  { id: 'environment', label: 'Environment' },
  { id: 'deployments', label: 'Deployments' },
  { id: 'logs', label: 'Logs', process: true },
]

// Addresses from when these were parts of their own, so a link kept from
// then still opens where the thing now is.
const MOVED = { 'auto-deploy': 'deployments', danger: 'configuration' }

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
  const wanted = MOVED[asked] ?? asked
  const part = parts.some((p) => p.id === wanted) ? wanted : 'overview'

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
  const snapshotsTitle = service?.noSnapshot
    ? `Take a snapshot before every deployment of ${name}`
    : `Deploy ${name} without a snapshot`
  const toggleSnapshots = () => onWrite(snapshotsTitle, () => setWriting('snapshot-policy'))
  const repair = () => onWrite(`Repair the checkout of ${name}`, () => setWriting('repair'))
  const finished = (what) => async () => {
    await reload()
    setSettling(what)
  }

  // The one move the situation calls for, and the sentence that says why.
  // A stopped container stops everything in it, so starting it comes first;
  // a service that is down is looked at before it is written to; typed and
  // saved changes wait for a redeploy; and otherwise, redeploying is what
  // this page is for.
  const page = site ? servingOf(service) : null
  const is = site ? 'is serving' : 'is running'
  const situation = !up
    ? { sentence: `${container.name} is stopped, so ${name} is not running.`, primary: 'start-container' }
    : site && page.state !== 'active'
      ? { sentence: `${name} ${page.sentence}`, primary: 'redeploy' }
    : !running && !site
      ? { sentence: `${name} is ${service.state || 'not running'}${whole.length ? `, so ${whole[0].domain} answers with an error page` : ''}.`, primary: 'logs' }
      : changed
        ? { sentence: `${name} ${is}. Its configuration has changes that are not saved yet.`, primary: 'redeploy' }
      : service.pending
        ? { sentence: `${name} ${is}. Saved configuration changes are not deployed yet.`, primary: 'redeploy' }
        : whole.length === 0 && names.length > 0
          ? { sentence: `${name} ${is}, reached only at ${names.map((n) => n.domain).join(', ')}.`, primary: 'redeploy' }
        : whole.length === 0
          ? { sentence: noPort ? `${name} ${is}. It listens on no port, so no domain can reach it.` : `${name} ${is} on :${port}, and no domain reaches it yet.`, primary: noPort ? 'redeploy' : 'add-domain' }
          : {
              sentence: `${name} ${is} and answers at ${whole[0].domain}${
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

  const subject = `${container.name}/${name}`
  // The tab counts what it is named for. It counted snapshots, and said 2
  // over a history that said none.
  const deployCount = (jobsReading?.jobs ?? []).filter((j) => j.subject === subject && DEPLOYS.has(j.kind)).length
  const badges = {
    configuration: changed ? { dot: true, sr: 'not saved' } : null,
    domains: { text: String(names.length) },
    deployments: deployCount ? { text: String(deployCount), sr: `${deployCount} deployments` } : null,
  }

  return (
    <>
      {/* Who, how it is, and what to do about it: the same on every part. */}
      <header className="space-y-1.5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2.5">
            <h1 className="truncate font-mono text-base">{name}</h1>
            <StatePill up={up} state={service.state} page={page} />
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
            {verbs.filter((v) => v.key !== 'stop').map((v) => (
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
            {/* Stopping takes the service down for everybody using it. It sat
                between Restart and Redeploy, one slip from either; it is a
                menu away instead, which is still one click and a look. */}
            {up && verbs.some((v) => v.key === 'stop') && (
              <Actions folded label={`More for ${name}`} actions={verbs.filter((v) => v.key === 'stop').map((v) => ({ label: `${v.label}…`, onClick: v.onClick, danger: true }))} />
            )}
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

      <Tabs parts={parts} part={part} to={to} badges={badges} label={`${name}: sections`} />

      <div className="min-w-0 space-y-4">
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

        {part === 'deployments' && (
          <>
            {/* First, not after the history: with a long list of deployments
                below them, the settings that decide what every one of them
                does were a scroll away from anybody looking for them. */}
            <div className="grid gap-4 xl:grid-cols-2 xl:items-start">
              <AutoDeploy service={service} state={autoDeploy} agentDown={agentDown} />
              <SnapshotPolicy name={name} off={Boolean(service.noSnapshot)} onToggle={toggleSnapshots} />
            </div>
            <Activity
              subject={subject}
              title={`What was done to ${name}`}
              reading={jobsReading}
              commandMode={commandMode}
              empty={`Nothing has been done to ${name} from this panel yet.`}
            />
            <Snapshots
              only={name}
              untaken={Boolean(service.noSnapshot)}
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
            <Checkout path={service.path} onRepair={repair} />
          </>
        )}

        {part === 'logs' && (
          <section aria-label={`Logs of ${name}`} className="flex h-[70vh] min-h-80 flex-col overflow-hidden rounded-lg border border-edge bg-panel">
            <LogPane container={container.name} service={name} title="Logs" />
          </section>
        )}

        {/* At the end of what is set, apart: removing it is the last thing
            about it that can be changed. */}
        {part === 'configuration' && (
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

      {writing === 'repair' && (
        <PlanDialog
          request={{
            title: `Repair the checkout of ${name}`,
            url: `/api/hosts/local/instances/${container.name}/services/${name}/repair`,
            method: 'POST',
            verb: 'Repair',
          }}
          onClose={() => setWriting(null)}
          onFinished={reload}
        />
      )}

      {writing === 'snapshot-policy' && (
        <PlanDialog
          request={{
            title: snapshotsTitle,
            url: `/api/hosts/local/instances/${container.name}/services/${name}/snapshot-policy`,
            method: 'PUT',
            defaults: { enabled: Boolean(service.noSnapshot) },
            verb: service.noSnapshot ? 'Turn on' : 'Turn off',
          }}
          onClose={() => setWriting(null)}
          onFinished={reload}
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
// A site's word comes from asking for its page, since it has no process.
function StatePill({ up, state, page }) {
  if (!up) return <Status status={{ label: 'container stopped', tone: 'muted' }} />
  if (page) return <Status status={{ label: page.label, tone: toneOf(page), explain: `Its front page ${page.sentence}` }} />
  if (state === 'active') return <Status status={{ label: 'running', tone: 'running' }} />
  return <Status status={{ label: state || 'unknown', tone: 'problem' }} />
}

// What it is and where it stands, on one screen: the facts somebody asks for
// first, each one copyable; how it is reached; what was done to it last.
// Whether deployments start with a snapshot, beside the snapshots themselves:
// the choice and what it leaves are read in one place. Said as a cost both
// ways, because each is one — a full copy of the container every time, or no
// way back for anything outside the code.
function SnapshotPolicy({ name, off, onToggle }) {
  return (
    <Card
      title="Snapshot before each deployment"
      action={
        <button type="button" onClick={onToggle} className={SECONDARY}>
          {off ? 'Turn on…' : 'Turn off…'}
        </button>
      }
    >
      <p className="px-4 py-3 text-xs">
        {off ? (
          <span className="text-caution">
            Off. {name} deploys without a copy of the container first: if a deployment breaks something, the way
            back is redeploying an older commit, and whatever changed outside the code — the database, files — has
            none. The snapshots below were taken before this was turned off.
          </span>
        ) : (
          <span className="text-muted">
            On. Every deployment of {name}, and every change to its environment, starts with a copy of the whole
            container to come back to. Depending on how the host stores containers, that copy can be the size of the
            container itself.
          </span>
        )}
      </p>
    </Card>
  )
}

// Last and quiet: it is only ever needed after a deployment failed and said to
// come here. Offered always rather than only then, because the panel cannot
// tell a damaged checkout without asking git, and asking is the repair's job.
function Checkout({ path, onRepair }) {
  return (
    <Card
      title="Copy of the repository"
      action={
        <button type="button" onClick={onRepair} className={SECONDARY}>
          Repair…
        </button>
      }
    >
      <p className="px-4 py-3 text-xs text-muted">
        Kept in {path}. If a deployment stops because git&apos;s files there are damaged — what a power cut while
        fetching leaves — repairing removes the empty files and fetches the branch again. Nothing outside .git
        changes; deploy again afterwards.
      </p>
    </Card>
  )
}

// Overview reads top to bottom as answers, each on its own row with its
// question on the left: where it is reached, how the last deployment went, what
// checks it, and the way back. Whether it runs is already in the header; saying
// it again here made two pills that could be read as two different things.
// The facts for copying come after, split by what they are about — they are
// looked up, not read.
function Overview({ service, container, port, names, certificates, noPort, autoDeploy, jobs, subject, to, stoppedId, onAddDomain }) {
  const site = service.adopted?.site
  const last = jobs.find((j) => j.subject === subject && DEPLOYS.has(j.kind))
  const auto = autoDeploy.data?.enabled
  const whole = names.filter((n) => !n.prefix)
  const first = whole[0] ?? names[0]
  const covering = first?.ssl ? servedWith(first, certificates) : null

  return (
    <>
      <Card title="At a glance">
        <dl className="divide-y divide-edge">
          <Row label="Reached at" action={<More href={to('domains')}>Manage</More>}>
            {first ? (
              <>
                <a
                  href={`${first.ssl ? 'https' : 'http'}://${first.domain}`}
                  target="_blank"
                  rel="noreferrer"
                  className="font-mono underline-offset-4 hover:underline"
                >
                  {first.domain}
                  <span className="sr-only"> (opens in a new tab)</span>
                </a>
                <Aside>
                  {!first.ssl ? 'http only, nothing encrypted' : covering ? (
                    <span className={covering.daysLeft < 7 ? 'text-problem' : covering.daysLeft < 21 ? 'text-caution' : ''}>
                      {covering.daysLeft < 0 ? 'certificate expired' : `certificate valid ${covering.daysLeft} more days`}
                    </span>
                  ) : 'https'}
                  {names.length > 1 && <>, and {names.length - 1} more</>}
                </Aside>
              </>
            ) : (
              <span className="text-muted">
                {noPort ?? (
                  <>
                    No domain reaches it yet.{' '}
                    <Gated reasonId={stoppedId} onClick={onAddDomain} className="text-ink underline underline-offset-4">
                      Add one
                    </Gated>
                  </>
                )}
              </span>
            )}
          </Row>

          <Row label="Last deployment" action={<More href={to('deployments')}>All</More>}>
            {last ? (
              <>
                <Chip pill {...outcomePill(last)} />
                <Aside>
                  {last.kind === 'auto-deploy' ? 'automatic, after a new commit' : 'from the panel'}, {ago(new Date(last.started))}
                </Aside>
                {last.status === 'failed' && last.error && (
                  <p className="mt-1 w-full text-problem">Stopped at step {last.reached} of {last.steps}: {last.error}</p>
                )}
              </>
            ) : (
              <span className="text-muted">
                None from this panel yet{service.commit ? <> — <span className="font-mono text-ink">{service.commit.slice(0, 7)}</span> is what runs</> : ''}.
              </span>
            )}
          </Row>

          <Row label="Automatic deployments" action={<More href={to('deployments')}>Change</More>}>
            {!autoDeploy.read ? (
              <span className="text-faint">…</span>
            ) : auto ? (
              <>
                On <Aside>every new commit on <span className="font-mono text-ink">{service.branch || 'its branch'}</span> is deployed</Aside>
              </>
            ) : (
              <>
                Off <Aside>deployed only from here</Aside>
              </>
            )}
          </Row>

          {!site && (
            <Row label="Readiness check" action={<More href={to('configuration')}>{service.health?.path ? 'Change' : 'Set one'}</More>}>
              {service.health?.path ? (
                <>
                  <span className="font-mono">{service.health.path}</span>
                  <Aside>{service.good ? 'answered at the last deployment that worked' : 'asked at each deployment'}</Aside>
                </>
              ) : (
                <span className="text-caution">None — a deployment counts as done when the unit starts, answering or not.</span>
              )}
            </Row>
          )}

          <Row label="Way back">
            <WayBack service={service} site={site} />
          </Row>
        </dl>

        {/* The fact belongs here; what to do about it, beside it. */}
        {service.noSnapshot && (
          <p className="border-t border-caution/30 bg-caution/5 px-4 py-3 text-sm text-caution">
            Deployed without snapshots. A deployment that breaks something is undone by redeploying an older commit;
            the database and files have no way back.{' '}
            <a href={to('deployments')} className="underline underline-offset-2">Snapshot settings</a>
          </p>
        )}
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Source" action={<More href={to('configuration')}>Edit</More>}>
          <dl className="divide-y divide-edge">
            <Fact label="Repository" value={service.repo} copy />
            <Fact label="Branch" value={service.branch} />
            <Fact label="Deployed commit" value={service.commit ? service.commit.slice(0, 7) : null} copy={service.commit} />
          </dl>
        </Card>
        <Card title="On the container">
          <dl className="divide-y divide-edge">
            <Fact label="Runtime" value={service.runtime || null} />
            {site ? (
              <Fact label="Published to" value={site} copy />
            ) : (
              service.adopted?.unit && <Fact label="Runs as" value={service.adopted.unit} />
            )}
            <Fact label="Code in" value={service.path} copy />
            <Fact label="Internal address" value={port ? `${container.address}:${port}` : container.address} copy />
          </dl>
        </Card>
      </div>
    </>
  )
}

// One question and its answer. The question keeps a column of its own on a
// wide screen so the answers line up and can be read down without the labels
// in the way; on a phone it sits above.
function Row({ label, action, children }) {
  return (
    <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:gap-4">
      <dt className="shrink-0 text-sm text-muted sm:w-44">{label}</dt>
      <dd className="flex min-w-0 flex-1 items-center justify-between gap-4">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm">{children}</div>
        {action && <div className="shrink-0">{action}</div>}
      </dd>
    </div>
  )
}

function Aside({ children }) {
  return <span className="text-muted">· {children}</span>
}

function More({ href, children }) {
  return (
    <a href={href} className="text-xs text-muted transition hover:text-ink">
      {children} →
    </a>
  )
}

function outcomePill(job) {
  const outcome = outcomeOf(job)
  const dot = { running: 'bg-caution animate-pulse', failed: 'bg-problem' }[job.status]
  return { label: outcome.label, tone: outcome.tone, explain: outcome.explain, dot }
}

// WayBack is what a broken deployment would be undone with. It is the last
// commit that passed its check — recorded with or without snapshots — and,
// before croft recorded that, the snapshot named after it. A snapshot older
// than what runs is said to be older: shown plainly as "known good", one from
// weeks back read as the version to return to.
function WayBack({ service, site }) {
  if (service.good) {
    const current = service.commit && service.commit === service.good
    return (
      <>
        <span className="font-mono">{service.good.slice(0, 7)}</span>
        <CopyButton text={service.good} label="Copy the commit that last worked" />
        <Aside>
          {current ? 'what runs now, it passed its check' : 'the last commit that passed its check; redeploy it to go back'}
          {service.goodAt ? `, ${ago(new Date(service.goodAt))}` : ''}
        </Aside>
      </>
    )
  }
  if (service.healthy) {
    const taken = snapshotDate(service.healthy)
    return (
      <>
        <span>Snapshot{taken ? ` from ${ago(taken)}` : ''}</span>
        <CopyButton text={service.healthy} label="Copy the snapshot's name" />
        <span className={service.noSnapshot ? 'text-caution' : 'text-muted'}>
          ·{' '}
          {service.noSnapshot
            ? 'older than what runs — restoring it loses everything since'
            : 'taken before the last deployment that worked'}
        </span>
      </>
    )
  }
  return (
    <span className="text-muted">
      Nothing recorded yet —{' '}
      {site ? 'the next deployment that publishes is the first.' : 'the next deployment that passes its readiness check is.'}
    </span>
  )
}

// snapshotDate reads the moment out of croft-deploy-<service>-YYYYMMDD-HHMMSS.
function snapshotDate(name) {
  const m = /-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/.exec(name ?? '')
  return m ? new Date(`${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}`) : null
}

// One fact for copying: its value in full on hover, and a copy button when it
// is something pasted elsewhere.
function Fact({ label, value, copy, none = 'none' }) {
  const text = typeof copy === 'string' ? copy : value
  return (
    <Row label={label}>
      {value ? (
        <span className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 truncate font-mono" title={String(value)}>{value}</span>
          {copy && <CopyButton text={String(text)} label={`Copy the ${label.toLowerCase()}`} className="shrink-0" />}
        </span>
      ) : (
        <span className="text-faint">{none}</span>
      )}
    </Row>
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
              // Empty, the way forward is here rather than up in the card's
              // header, where nobody reading this line was looking.
              <div className="space-y-2">
                <p className="text-muted">
                  No variables yet. Database addresses, keys and settings {name} reads from its environment go here.
                </p>
                <button onClick={onEdit} className={RAISED}>
                  Add variables&hellip;
                </button>
              </div>
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

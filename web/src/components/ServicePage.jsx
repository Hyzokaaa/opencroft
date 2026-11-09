import { useEffect, useId, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Gated from './Gated.jsx'
import Actions from './Actions.jsx'
import CopyButton from './CopyButton.jsx'
import PlanDialog from './PlanDialog.jsx'
import DomainList from './DomainList.jsx'
import Activity, { DEPLOYS, ago, duration, outcomeOf } from './Activity.jsx'
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
          ? { sentence: `${name} ${is}, reached only at ${names.map((n) => n.domain).join(', ')}.`, primary: 'redeploy', normal: true }
        : whole.length === 0
          ? { sentence: noPort ? `${name} ${is}. It listens on no port, so no domain can reach it.` : `${name} ${is} on :${port}, and no domain reaches it yet.`, primary: noPort ? 'redeploy' : 'add-domain' }
          : {
              sentence: `${name} ${is} and answers at ${whole[0].domain}${
                whole.length > 1 ? ` and ${whole.length - 1} other domain${whole.length > 2 ? 's' : ''}` : ''
              }.`,
              primary: 'redeploy',
              normal: true,
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

        {/* The sentence is for when something needs saying. Running and
            reached is what the pill and the line below already show; said a
            third time it was one more thing to read before what mattered. */}
        {(!situation.normal || changed) && <p className="text-sm">
          {situation.sentence}
          {changed && part !== 'configuration' && (
            <>
              {' '}
              <a href={to('configuration')} className="text-yours underline underline-offset-4">Review the changes</a>
            </>
          )}
        </p>}

        <p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted">
          {(whole[0] ?? names[0]) && (
            <a
              href={`${(whole[0] ?? names[0]).ssl ? 'https' : 'http'}://${(whole[0] ?? names[0]).domain}`}
              target="_blank"
              rel="noreferrer"
              className="font-mono text-ink underline-offset-4 hover:underline"
            >
              {(whole[0] ?? names[0]).domain} ↗<span className="sr-only"> (opens in a new tab)</span>
            </a>
          )}
          <span>
            in{' '}
            <a href={href('containers', container.name)} className="font-mono text-ink underline-offset-4 hover:underline">
              {container.name}
            </a>
          </span>
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
            onToggleSnapshots={toggleSnapshots}
            onRollback={onRollback}
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

// Overview is what is in production and how it got there, after Vercel's
// project page: one card with the four facts asked first — where it is
// reached, whether it is well, when it was deployed, from what — and the
// deployments under it. How it is set up lives in Configuration and nowhere
// here: twice, a page that mixed the two had nothing that stood out.
function Overview({ service, container, names, certificates, noPort, jobs, subject, to, stoppedId, onAddDomain, onToggleSnapshots, onRollback }) {
  const site = service.adopted?.site
  const up = container.status === 'running'
  const deploys = jobs.filter((j) => j.subject === subject && DEPLOYS.has(j.kind))
  const last = deploys[0]
  const whole = names.filter((n) => !n.prefix)
  const first = whole[0] ?? names[0]
  const covering = first?.ssl ? servedWith(first, certificates) : null
  // Restoring is offered only when it is a way back: with snapshots off, the
  // one left is older than what runs, and restoring it loses everything since.
  const restorable = !service.noSnapshot && service.healthy

  return (
    <div className="max-w-5xl space-y-4">
      <Card
        title="In production"
        action={
          restorable && (
            <Gated reasonId={stoppedId} onClick={() => onRollback(container, service.healthy)} className={SECONDARY}>
              Go back…
            </Gated>
          )
        }
      >
        <dl className="grid gap-x-8 gap-y-5 px-5 py-5 sm:grid-cols-2">
          <div className="min-w-0">
            <dt className="text-xs text-muted">Domain</dt>
            <dd className="mt-1">
              {first ? (
                <>
                  <a
                    href={`${first.ssl ? 'https' : 'http'}://${first.domain}`}
                    target="_blank"
                    rel="noreferrer"
                    className="block truncate font-mono text-sm underline-offset-4 hover:underline"
                    title={first.domain}
                  >
                    {first.domain} ↗<span className="sr-only"> (opens in a new tab)</span>
                  </a>
                  <p className={`mt-0.5 text-xs ${!first.ssl ? 'text-caution' : covering && covering.daysLeft < 7 ? 'text-problem' : covering && covering.daysLeft < 21 ? 'text-caution' : 'text-muted'}`}>
                    {!first.ssl ? 'http only' : !covering ? 'https' : covering.daysLeft < 0 ? 'https · certificate expired' : `https · certificate ${covering.daysLeft} days`}
                    {names.length > 1 && (
                      <a href={to('domains')} className="text-muted hover:text-ink"> · {names.length - 1} more</a>
                    )}
                  </p>
                </>
              ) : (
                <p className="text-sm text-muted">
                  {noPort ?? (
                    <>
                      None yet.{' '}
                      <Gated reasonId={stoppedId} onClick={onAddDomain} className="text-ink underline underline-offset-4">
                        Add one
                      </Gated>
                    </>
                  )}
                </p>
              )}
            </dd>
          </div>

          <div className="min-w-0">
            <dt className="text-xs text-muted">Status</dt>
            <dd className="mt-1 text-sm">
              <Health up={up} service={service} site={site} last={last} container={container} to={to} />
            </dd>
          </div>

          <div className="min-w-0">
            <dt className="text-xs text-muted">Deployed</dt>
            <dd className="mt-1 text-sm">
              {last ? (
                <>
                  <span title={new Date(last.started).toLocaleString()}>{ago(new Date(last.started))}</span>
                  <span className="text-muted">
                    {' '}· {last.kind === 'auto-deploy' ? 'automatic, after a push' : 'from the panel'}
                    {last.ended && ` · took ${duration(new Date(last.started), new Date(last.ended))}`}
                  </span>
                </>
              ) : (
                <span className="text-muted">Not from this panel yet</span>
              )}
            </dd>
          </div>

          <div className="min-w-0">
            <dt className="text-xs text-muted">Source</dt>
            <dd className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm">
              {service.branch && <span className="font-mono">{service.branch}</span>}
              {service.branch && service.commit && <span className="text-muted">·</span>}
              {service.commit && (
                <>
                  <span className="font-mono">{service.commit.slice(0, 7)}</span>
                  <CopyButton text={service.commit} label="Copy the commit that runs" />
                </>
              )}
              {!service.branch && !service.commit && <span className="text-muted">unknown</span>}
            </dd>
            {/* The last commit that passed its check, when it is not what
                runs: what a broken deployment is undone with. */}
            {service.good && service.good !== service.commit && (
              <p className="mt-0.5 text-xs text-muted">
                last that passed its check: <span className="font-mono text-ink">{service.good.slice(0, 7)}</span>
              </p>
            )}
          </div>
        </dl>
      </Card>

      {service.noSnapshot && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-caution/30 bg-caution/5 px-5 py-3">
          <p className="min-w-0 flex-1 text-sm">
            <span className="font-medium text-caution">No snapshots.</span>{' '}
            <span className="text-muted">Going back restores the code, not the database or files.</span>
          </p>
          <button type="button" onClick={onToggleSnapshots} className={SECONDARY}>
            Turn on…
          </button>
        </div>
      )}

      <Card
        title="Deployments"
        action={
          deploys.length > 0 && (
            <a href={to('deployments')} className="text-xs text-muted transition hover:text-ink">
              All {deploys.length} →
            </a>
          )
        }
      >
        {deploys.length === 0 ? (
          <p className="px-5 py-6 text-sm text-muted">
            None from this panel yet. Redeploy makes the first, and every one after shows here.
          </p>
        ) : (
          <ul className="divide-y divide-edge">
            {deploys.slice(0, 5).map((j, i) => {
              // The newest one that ended well is what serves; one before it
              // was replaced, and a failure after it is said as a failure.
              const live = i === 0 && j.status === 'done'
              const failed = j.status === 'failed'
              return (
                <li key={j.id} className={`flex items-center gap-3 px-5 py-2.5 text-sm ${failed ? 'bg-problem/[0.04]' : ''}`}>
                  <span className="w-24 shrink-0">
                    {live ? <Chip pill label="Live" tone="border-running/40 text-running" dot="bg-running" /> : <Chip pill {...outcomePill(j)} />}
                  </span>
                  <span className="hidden w-32 shrink-0 text-muted sm:block">{j.kind === 'auto-deploy' ? 'automatic' : 'from the panel'}</span>
                  <span className={`min-w-0 flex-1 truncate ${failed ? 'text-problem' : 'text-muted'}`} title={failed ? j.error : undefined}>
                    {failed ? j.error || `stopped at step ${j.reached} of ${j.steps}` : j.ended ? `took ${duration(new Date(j.started), new Date(j.ended))}` : 'running…'}
                  </span>
                  <span className="shrink-0 text-xs text-muted" title={new Date(j.started).toLocaleString()}>
                    {ago(new Date(j.started))}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </div>
  )
}

// Health is the status in a word and, beside it, what that word rests on:
// the unit's state, and whether the last deployment's check answered.
function Health({ up, service, site, last, container, to }) {
  const dot = (tone) => <span className={`mr-1.5 inline-block size-2 rounded-full ${tone}`} aria-hidden="true" />
  if (!up) return <>{dot('bg-stopped')}Stopped <span className="text-muted">· {container.name} is stopped</span></>
  if (site) {
    const page = servingOf(service)
    return <>{dot(page.state === 'active' ? 'bg-running' : 'bg-problem')}{page.label} <span className="text-muted">· its front page {page.sentence}</span></>
  }
  if (service.state !== 'active') {
    return (
      <>
        {dot('bg-problem')}
        <span className="text-problem">{service.state ? service.state[0].toUpperCase() + service.state.slice(1) : 'Not running'}</span>{' '}
        <a href={to('logs')} className="text-muted underline underline-offset-4">see the logs</a>
      </>
    )
  }
  if (last?.status === 'failed') {
    return (
      <>
        {dot('bg-caution')}Running <span className="text-caution">· the last deployment failed</span>
        {last.error && <p className="mt-0.5 truncate text-xs text-problem" title={last.error}>{last.error}</p>}
      </>
    )
  }
  if (!service.health?.path) {
    return (
      <>
        {dot('bg-running')}Running{' '}
        <a href={to('configuration')} className="text-muted underline underline-offset-4">· nothing checks it answers</a>
      </>
    )
  }
  return (
    <>
      {dot('bg-running')}Ready{' '}
      <span className="text-muted">
        · <span className="font-mono">{service.health.path}</span> {last?.verified ? 'answered' : 'asked at each deployment'}
      </span>
    </>
  )
}

function outcomePill(job) {
  const outcome = outcomeOf(job)
  const dot = { running: 'bg-caution animate-pulse', failed: 'bg-problem' }[job.status]
  return { label: outcome.label, tone: outcome.tone, explain: outcome.explain, dot }
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

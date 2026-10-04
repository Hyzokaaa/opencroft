import { useEffect, useState } from 'react'
import PlanDialog from './PlanDialog.jsx'
import { Field, Frame, split } from './DeployDialog.jsx'
import { readJSON, humane } from '../lib/api.js'

// Taking on something croft found: a unit running, or a site its web server
// serves. What it can read off the machine is shown as fact — where the code
// is, who owns it, where it came from — and only what nothing on the machine
// says is asked: how to build it.
//
// Adopting changes nothing inside the container. The plan it leads to is a
// list of notes croft writes down; the first change is the next deployment.
export default function AdoptDialog({ container, subject, onClose, onFinished }) {
  const [found, setFound] = useState(null)
  const [error, setError] = useState(null)
  const [answer, setAnswer] = useState(null)
  const [adopting, setAdopting] = useState(false)

  const isSite = subject.kind === 'sites'
  const base = `/api/hosts/local/instances/${container.name}/${subject.kind}/${subject.name}`

  useEffect(() => {
    let current = true
    fetch(`${base}/adoption`)
      .then(readJSON)
      .then((payload) => {
        if (!current) return
        setFound(payload)
        setAnswer({
          name: serviceName(isSite ? repoName(payload.repo) || subject.name : subject.name),
          install: (payload.install ?? []).join(' && '),
          build: (payload.build ?? []).join(' && '),
          port: payload.port ?? 0,
          health: '',
          contains: '',
        })
      })
      .catch((e) => current && setError(humane(e)))
    return () => {
      current = false
    }
  }, [base, isSite, subject.name])

  if (adopting) {
    return (
      <PlanDialog
        request={{
          title: `Adopt ${subject.name}`,
          url: `${base}/adopt`,
          method: 'POST',
          defaults: {
            name: answer.name.trim(),
            install: split(answer.install),
            build: split(answer.build),
            port: isSite ? 0 : Number(answer.port) || 0,
            health: isSite
              ? { path: '', contains: '', status: 0 }
              : { path: answer.health.trim(), contains: answer.contains.trim(), status: 0 },
          },
        }}
        onClose={onClose}
        dirty
        onBack={() => setAdopting(false)}
        onFinished={onFinished}
      />
    )
  }

  const set = (key) => (value) => setAnswer({ ...answer, [key]: value })

  return (
    <Frame title={`Adopt ${subject.name}`} onClose={onClose}>
      <div className="space-y-4 px-5 py-4">
        {error && <p role="alert" className="text-xs text-problem">{error}</p>}
        {!found && !error && (
          <p className="text-xs text-muted">
            {isSite
              ? 'Reading the web server and looking for the checkout that built this site…'
              : 'Reading the unit and its checkout…'}
          </p>
        )}

        {found && (
          <>
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-xs">
              {isSite ? (
                <>
                  <Fact label="Answers" value={found.domains?.join(', ')} />
                  <Fact label="Served from" value={found.site} note="the web server's config stays as it is" />
                  <Fact label="Built in" value={found.path ? `${found.path}/${found.output}` : '—'}
                    note={found.path ? 'its index.html is the one being served' : ''} />
                </>
              ) : (
                <>
                  <Fact label="Unit" value={found.unit} note="stays as it is" />
                  <Fact label="Code" value={found.path} />
                  <Fact label="Runs as" value={found.runAs || 'root'} />
                  <Fact label="Runs" value={found.runs} />
                </>
              )}
              <Fact
                label="Environment"
                value={found.envFile || 'none'}
                note={isSite ? 'read by the build — croft never writes it' : 'yours — croft never writes it'}
              />
              <Fact
                label="Follows"
                value={found.repo ? `${found.repo} @ ${found.branch}` : '—'}
                note={found.commit ? `at ${found.commit.slice(0, 7)}` : ''}
              />
            </dl>

            {found.problem ? (
              <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
                Croft cannot take this on: {found.problem}.
              </p>
            ) : (
              <>
                {/* Said before anybody agrees, not discovered after: the one
                    thing a deployment of this checkout takes away. */}
                {found.changed?.length > 0 && (
                  <p className="rounded border border-caution/30 bg-caution/[0.06] px-3 py-2 text-xs text-caution">
                    Deploying resets the checkout to {found.branch}, so these edits made in place
                    would be lost: <span className="font-mono">{found.changed.join(', ')}</span>. Files
                    git does not track stay.
                  </p>
                )}

                <p className="text-xs text-faint">
                  {found.runtime ? `Detected ${found.runtime}. ` : ''}
                  How to build it is the one thing nothing on the machine says. These are
                  suggestions — the runtime is already installed, so they are only the project&apos;s
                  own steps.
                  {isSite && ` Deploying publishes ${found.output}/ to ${found.site} — nothing is restarted.`}
                </p>

                <Field label="Call it" value={answer.name} onChange={set('name')}
                  hint={isSite ? 'Names it in croft and its snapshots.' : 'Names it in croft and its snapshots. The unit keeps its own name.'} />
                <Field label="Install — has to finish" value={answer.install} onChange={set('install')} mono
                  hint="Run in the checkout, as root. Several commands joined with &&." />
                <Field label="Build — has to finish" value={answer.build} onChange={set('build')} mono
                  hint={isSite ? `Has to leave the site in ${found.output}/.` : 'Left empty, no build step happens.'} />

                {!isSite && (
                  <div className="grid gap-3 sm:grid-cols-3">
                    <Field label="Port it listens on" type="number" value={answer.port} onChange={set('port')} />
                    <Field label="Ready when this answers" value={answer.health} onChange={set('health')} mono
                      placeholder="/health" />
                    <Field label="…and the body contains" value={answer.contains} onChange={set('contains')} mono
                      placeholder="optional" />
                  </div>
                )}
              </>
            )}
          </>
        )}
      </div>

      <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
        <p className="text-xs text-faint">Nothing inside the container changes until the next deployment.</p>
        <button
          onClick={() => setAdopting(true)}
          disabled={!found || Boolean(found.problem) || !answer?.name.trim()}
          className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
        >
          Show me the plan
        </button>
      </footer>
    </Frame>
  )
}

function Fact({ label, value, note }) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0 truncate font-mono">
        {value}
        {note && <span className="ml-2 font-sans text-faint">{note}</span>}
      </dd>
    </>
  )
}

function repoName(repo) {
  return (repo ?? '').replace(/\/+$/, '').split('/').pop().replace(/\.git$/, '')
}

// A unit or a domain may be called things a service may not; a service name
// is lowercase letters, digits and dashes, because it ends up in snapshot
// names.
function serviceName(from) {
  return from
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^[^a-z0-9]+/, '')
    .slice(0, 31)
}

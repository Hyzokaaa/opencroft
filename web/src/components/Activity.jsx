import { useCallback, useEffect, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import { Progress } from './PlanDialog.jsx'

// What croft did, newest first — the one record the machine itself cannot give
// back. A container shows what it runs now; this is where a deployment that
// failed an hour ago is still visible, with the step it stopped at and why.
export default function Activity({ commandMode }) {
  const { jobs, error, read } = useJobs()
  const [scope, setScope] = useState('deploys')
  const [failedOnly, setFailedOnly] = useState(false)
  const [open, setOpen] = useState(null)

  const shown = jobs.filter(
    (j) => (scope === 'all' || j.kind === 'deploy') && (!failedOnly || j.status === 'failed'),
  )
  const failures = jobs.filter((j) => (scope === 'all' || j.kind === 'deploy') && j.status === 'failed').length

  return (
    <Card
      title={scope === 'deploys' ? 'Deployments' : 'Everything croft did'}
      count={read ? shown.length : undefined}
      commandMode={commandMode}
      commands={['sqlite3 /var/lib/croft/croft.db "SELECT started, kind, subject, status FROM jobs ORDER BY started DESC"']}
      action={
        <div className="flex items-center gap-1.5">
          <Toggle on={scope === 'deploys'} onClick={() => setScope('deploys')}>Deploys</Toggle>
          <Toggle on={scope === 'all'} onClick={() => setScope('all')}>Everything</Toggle>
          <span className="mx-1 h-4 w-px bg-edge" />
          <Toggle on={failedOnly} onClick={() => setFailedOnly(!failedOnly)}>
            Failed{failures ? ` (${failures})` : ''}
          </Toggle>
        </div>
      }
    >
      {error && <p className="px-4 py-3 text-xs text-problem">{error}</p>}

      {!read ? (
        <p role="status" className="px-4 py-8 text-center text-xs text-muted">Reading the history&hellip;</p>
      ) : shown.length === 0 ? (
        <p className="px-4 py-8 text-center text-xs text-muted">
          {failedOnly ? 'Nothing here has failed.' : 'Nothing has been done here yet.'}
        </p>
      ) : (
        <ul className="divide-y divide-edge">
          {shown.map((j) => (
            <Row key={j.id} job={j} open={open === j.id} onToggle={() => setOpen(open === j.id ? null : j.id)} />
          ))}
        </ul>
      )}
    </Card>
  )
}

const OUTCOME = {
  running: { label: 'running', tone: 'border-caution/40 text-caution' },
  done: { label: 'worked', tone: 'border-running/40 text-running' },
  failed: { label: 'failed', tone: 'border-problem/40 text-problem' },
  interrupted: {
    label: 'interrupted',
    tone: 'border-edge-strong text-muted',
    explain:
      'The panel restarted while this was running, so how it ended was never seen. What it did is on the machine, and the snapshot it took first is still there.',
  },
}

const KIND = {
  deploy: 'Deploy', rollback: 'Restore', destroy: 'Remove', release: 'Let go of', adopt: 'Adopt',
  restart: 'Restart', stop: 'Stop', start: 'Start', create: 'Create', route: 'Add domain',
  'route-edit': 'Edit domain', 'route-remove': 'Remove domain', tls: 'Enable https',
  expose: 'Expose the panel', database: 'Add database', 'database-remove': 'Remove database',
}

function Row({ job, open, onToggle }) {
  const outcome = OUTCOME[job.status] ?? { label: job.status, tone: 'border-edge text-muted' }
  const [container, service] = job.subject.split('/')
  const failed = job.status === 'failed'

  return (
    <li className={`relative ${failed ? 'bg-problem/[0.04]' : ''}`}>
      {failed && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}

      <div className="px-4 py-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Chip label={outcome.label} tone={outcome.tone} explain={outcome.explain} />
            <span className="text-sm">{KIND[job.kind] ?? job.kind}</span>
            <span className="font-mono text-sm">{service ?? container}</span>
            {service && <span className="font-mono text-xs text-muted">in {container}</span>}
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <span className="text-xs text-muted" title={new Date(job.started).toLocaleString()}>
              {ago(new Date(job.started))}
              {job.ended && ` · took ${duration(new Date(job.started), new Date(job.ended))}`}
            </span>
            <button
              onClick={onToggle}
              aria-expanded={open}
              className="rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
            >
              {open ? 'Hide steps' : 'Steps'}
            </button>
          </div>
        </div>

        {failed && (
          <p className="mt-1.5 truncate text-xs text-problem">
            Stopped at step {job.reached} of {job.steps}: {job.error}
          </p>
        )}
      </div>

      {open && <Detail id={job.id} status={job.status} />}
    </li>
  )
}

// The whole story of one job, read from the daemon: the steps it walked, what
// each said, and where it stopped.
function Detail({ id, status }) {
  const [job, setJob] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    let current = true
    fetch(`/api/jobs/${id}`)
      .then(async (res) => {
        const payload = await res.json()
        if (!res.ok) throw new Error(payload.error ?? `The daemon answered ${res.status}`)
        if (current) setJob(payload)
      })
      .catch((e) => current && setError(e.message))
    return () => {
      current = false
    }
  }, [id, status])

  return (
    <div className="border-t border-edge bg-ground/40 px-4 py-4">
      {error && <p className="text-xs text-problem">{error}</p>}
      {!job && !error && <p className="text-xs text-muted">Reading what happened&hellip;</p>}
      {job && (
        <>
          <Progress
            steps={job.plan?.steps ?? []}
            events={job.events ?? []}
            error={job.status === 'failed' ? job.error : null}
            done={job.status === 'done'}
          />
          {job.status === 'interrupted' && (
            <p className="mt-3 text-xs text-muted">
              The panel restarted while this ran, so its end was never seen.
            </p>
          )}
        </>
      )}
    </div>
  )
}

function Toggle({ on, onClick, children }) {
  return (
    <button
      onClick={onClick}
      aria-pressed={on}
      className={`rounded border px-2 py-0.5 text-xs transition ${
        on ? 'border-edge-strong bg-raised text-ink' : 'border-transparent text-muted hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

// Read every few seconds, like everything else on the panel: a deployment that
// is running now should be seen finishing without a reload.
function useJobs() {
  const [jobs, setJobs] = useState([])
  const [error, setError] = useState(null)
  const [read, setRead] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await fetch('/api/jobs?limit=200')
      const payload = await res.json()
      if (!res.ok) throw new Error(payload.error ?? `The daemon answered ${res.status}`)
      setJobs(payload.jobs ?? [])
      setError(null)
      setRead(true)
    } catch (e) {
      setError(e.message)
    }
  }, [])

  useEffect(() => {
    load()
    const timer = setInterval(load, 5000)
    return () => clearInterval(timer)
  }, [load])

  return { jobs, error, read }
}

function ago(when) {
  const minutes = Math.round((Date.now() - when.getTime()) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  if (minutes < 60 * 24) return `${Math.round(minutes / 60)}h ago`
  return `${Math.round(minutes / 1440)}d ago`
}

function duration(from, to) {
  const seconds = Math.max(0, Math.round((to - from) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

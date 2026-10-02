import { useEffect, useId, useRef, useState } from 'react'
import { useDialog, dialogProps } from '../lib/useDialog.js'

// Three states, in order: fill in the form, read the plan, watch it run.
//
// The plan is the dialog — not a "details" disclosure tucked under an OK
// button. Approving a write means approving a list of commands you have read.
// onBack, when given, returns to whatever came before the plan — the form of a
// longer flow — with everything typed still there. A plan with fields of its
// own goes back to them without being told.
export default function PlanDialog({ request, onClose, onFinished, onResult, onBack }) {
  const [stage, setStage] = useState(request.fields ? 'form' : 'loading')
  const [values, setValues] = useState(request.defaults ?? {})
  const [plan, setPlan] = useState(null)
  const [events, setEvents] = useState([])
  const [error, setError] = useState(null)
  const [done, setDone] = useState(null)
  const [stopping, setStopping] = useState(false)
  const [confirmed, setConfirmed] = useState('')
  const [touched, setTouched] = useState({})
  const stream = useRef(null)
  const job = useRef(null)
  const formId = useId()
  const edited = stage === 'form' && JSON.stringify(values) !== JSON.stringify(request.defaults ?? {})
  const dialog = useDialog(onClose, { dirty: edited })
  const leave = () => (!edited || window.confirm('Discard your changes?')) && onClose()

  // A URL can depend on what was chosen: moving the container picked in the
  // form is a request about that container.
  const url = typeof request.url === 'function' ? request.url(values) : request.url

  useEffect(() => {
    if (stage === 'loading') askForPlan(values)
    return () => stream.current?.close()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function askForPlan(body) {
    setStage('loading')
    setError(null)
    try {
      const res = await fetch(`${url}${url.includes('?') ? '&' : '?'}plan=1`, {
        method: request.method,
        headers: { 'Content-Type': 'application/json' },
        body: sends(request.method) ? JSON.stringify(body) : undefined,
      })
      const payload = await res.json()
      if (!res.ok) {
        setError(payload.error ?? `The daemon answered ${res.status}`)
        setStage(request.fields ? 'form' : 'error')
        return
      }
      setPlan(payload)
      setStage('plan')
    } catch (e) {
      setError(e.message)
      setStage(request.fields ? 'form' : 'error')
    }
  }

  async function run() {
    // A few steps that answer with something — inspecting a repository — are
    // waited for rather than watched. There is nothing useful to narrate, and
    // what comes back is the point.
    setStage(request.immediate ? 'immediate' : 'running')
    setEvents([])
    try {
      const res = await fetch(url, {
        method: request.method,
        headers: { 'Content-Type': 'application/json' },
        body: sends(request.method) ? JSON.stringify(values) : undefined,
      })
      const payload = await res.json()
      if (!res.ok) {
        setError(payload.error ?? `The daemon answered ${res.status}`)
        setStage('error')
        return
      }

      if (request.immediate) {
        onResult?.(payload)
        return
      }

      // The work belongs to the daemon. Closing this only stops watching it —
      // stopping it is a separate button, because they are separate things.
      job.current = payload.jobId
      const source = new EventSource(`/api/jobs/${payload.jobId}/events`)
      stream.current = source

      source.onmessage = (message) => {
        const event = JSON.parse(message.data)
        setEvents((current) => [...current, event])
        if (event.failed) setError(event.text)
      }
      source.addEventListener('end', () => {
        source.close()
        setDone(true)
        onFinished?.()
      })

      // Losing the stream used to be swallowed, which left this sitting at
      // 0/N for ever with nothing said. The work is the daemon's, so the
      // honest thing is to go and ask it what happened.
      source.onerror = () => {
        source.close()
        if (!done) askTheDaemon(payload.jobId)
      }
    } catch (e) {
      setError(e.message)
      setStage('error')
    }
  }

  // askTheDaemon is what happens when the stream breaks. The job record holds
  // the whole story, so a lost connection costs the live narration and nothing
  // else — and a job that is still running says so rather than looking stuck.
  async function askTheDaemon(jobId) {
    try {
      const res = await fetch(`/api/jobs/${jobId}`)
      const job = await res.json()

      if (Array.isArray(job.events)) setEvents(job.events)

      if (job.status === 'failed') {
        setError(job.error ?? 'It stopped, and the daemon did not say why.')
        return
      }
      if (job.status === 'done') {
        setDone(true)
        onFinished?.()
        return
      }
      setError(
        'Lost contact with the daemon while watching this. It is still running — ' +
          'reopen the panel to see where it got to.',
      )
    } catch {
      setError('Lost contact with the daemon while watching this, and could not ask it what happened.')
    }
  }

  // Closing this window and stopping the work are different things, and until
  // now only one of them existed. Stopping leaves the machine halfway through
  // a plan — which is why it says so, and why the snapshot from step one is
  // worth mentioning at exactly that moment.
  async function stop() {
    setStopping(true)
    try {
      await fetch(`/api/jobs/${job.current}/cancel`, { method: 'POST' })
    } catch {
      setError('Could not reach the daemon to stop it.')
    }
  }

  const steps = plan?.plan?.steps ?? []
  const running = stage === 'running' && !done && !error
  const failed = stage === 'error' || (stage === 'running' && Boolean(error))
  const missing = (request.fields ?? []).filter(
    (f) => !f.optional && f.type !== 'checkbox' && (values[f.name] === '' || values[f.name] == null),
  )
  const invalid = (request.fields ?? []).filter((f) => !valid(f, values[f.name]))
  const ready = missing.length === 0 && invalid.length === 0
  // What is typed to confirm something that cannot be taken back. The plan
  // is still read in full; this only makes the click deliberate.
  const unconfirmed = Boolean(request.confirm) && confirmed.trim() !== request.confirm
  const back = onBack ?? (request.fields ? () => { setError(null); setStage('form') } : null)

  // What usually comes next, offered once this has worked: a container just
  // created is opened, a domain just added is put on https. A finished job
  // that only says "Finished." leaves the person to go and look for it.
  const next = done && !error && (typeof request.next === 'function' ? request.next(values) : request.next)

  function submit() {
    setTouched(Object.fromEntries((request.fields ?? []).map((f) => [f.name, true])))
    if (ready) askForPlan(values)
  }

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-10">
      <div {...dialogProps(dialog)} className="w-full max-w-2xl rounded-lg border border-edge bg-panel shadow-2xl outline-none">
        <header className="flex items-center justify-between border-b border-edge px-5 py-3">
          <h2 id={dialog.titleId} className="text-sm font-medium">{request.title}</h2>
          <button onClick={leave} className="text-muted hover:text-ink" aria-label="Close">
            ✕
          </button>
        </header>

        <div className="px-5 py-4">
          {stage === 'form' && (
            <Form
              id={formId}
              fields={request.fields}
              values={values}
              onChange={setValues}
              error={error}
              touched={touched}
              onBlur={(name) => setTouched((t) => ({ ...t, [name]: true }))}
              onSubmit={submit}
            />
          )}

          {stage === "loading" && (
            <p className="py-6 text-sm text-muted">Working out what this would do…</p>
          )}

          {stage === "immediate" && (
            <p className="py-6 text-sm text-muted">{request.working ?? "Working…"}</p>
          )}

          {stage === 'plan' && (
            <>
              <p className="text-sm">{plan.summary}</p>
              <p className="mt-1 text-xs text-muted">
                Nothing has happened yet. These are the commands, in order.
              </p>
              <StepList steps={steps} />
              {request.confirm && (
                <label className="mt-4 block">
                  <span className="mb-1 block text-xs text-muted">
                    This cannot be undone from here. Type{' '}
                    <span className="font-mono text-ink">{request.confirm}</span> to confirm.
                  </span>
                  <input
                    value={confirmed}
                    onChange={(e) => setConfirmed(e.target.value)}
                    autoFocus
                    autoComplete="off"
                    spellCheck={false}
                    className="w-full rounded border border-field-edge bg-ground px-3 py-2 font-mono text-sm outline-none transition focus:border-ink/40"
                  />
                </label>
              )}
            </>
          )}

          {stage === 'error' && !steps.length && (
            <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
              {error}
            </p>
          )}

          {(stage === 'running' || (stage === 'error' && steps.length > 0)) && (
            <Progress steps={steps} events={events} error={error} done={done} />
          )}
        </div>

        <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
          <div className="flex min-w-0 items-center gap-3">
            {back && (stage === 'plan' || (stage === 'error' && !steps.length)) && (
              <button
                onClick={back}
                className="rounded border border-edge px-3 py-1.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
              >
                &larr; Back
              </button>
            )}
            <p className="text-xs text-muted">
              {stage === 'form' && !ready &&
                (missing.length
                  ? `Fill in ${missing.map((f) => f.label).join(', ')}`
                  : `Check ${invalid.map((f) => f.label).join(', ')}`)}
              {running &&
                'Stopping leaves the plan half applied. The snapshot from step one is still there.'}
              {failed && stage === 'running' && 'The steps before the failure ran; nothing after it did.'}
            </p>
          </div>

          <div className="flex shrink-0 gap-2">
            <button
              onClick={leave}
              className="rounded border border-edge px-3 py-1.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
            >
              {done || failed ? 'Close' : running ? 'Leave it running' : 'Cancel'}
            </button>

            {running && (
              <button
                onClick={stop}
                disabled={stopping}
                className="rounded border border-problem/50 bg-problem/10 px-3 py-1.5 text-xs text-problem transition hover:bg-problem/15 disabled:opacity-40"
              >
                {stopping ? 'Stopping…' : 'Stop it'}
              </button>
            )}

            {next && (
              <button
                onClick={next.onClick}
                autoFocus
                className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30"
              >
                {next.label}
              </button>
            )}

            {stage === 'form' && (
              <button
                type="submit"
                form={formId}
                // A checkbox left unticked and a field marked optional are answers too.
                disabled={!ready}
                className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
              >
                Show me the plan
              </button>
            )}

            {stage === 'plan' && (
              <button
                onClick={run}
                disabled={unconfirmed}
                className={`rounded border px-3 py-1.5 text-xs transition disabled:opacity-40 ${
                  request.destructive
                    ? 'border-problem/50 bg-problem/10 text-problem hover:bg-problem/15'
                    : 'border-edge-strong bg-raised hover:border-ink/30'
                }`}
              >
                {request.verb ?? 'Run'} · {steps.length} command{steps.length === 1 ? '' : 's'}
              </button>
            )}
          </div>
        </footer>
      </div>
    </div>
  )
}

// valid checks a field against the shape it declares, so a mistake is said
// beside the field when it is left rather than by the daemon a round trip later.
function valid(field, value) {
  if (!field.pattern || value === '' || value == null) return true
  return new RegExp(field.pattern).test(String(value))
}

function Form({ id, fields, values, onChange, error, touched, onBlur, onSubmit }) {
  return (
    <form
      id={id}
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
      className="space-y-3"
    >
      {fields.map((field) =>
        field.type === 'checkbox' ? (
          <label key={field.name} className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={Boolean(values[field.name])}
              onChange={(e) => onChange({ ...values, [field.name]: e.target.checked })}
              className="mt-0.5"
            />
            <span>
              <span className="block text-xs">{field.label}</span>
              {field.hint && <span className="block text-xs text-muted">{field.hint}</span>}
            </span>
          </label>
        ) : (
        <label key={field.name} className="block">
          <span className="mb-1 block text-xs text-muted">
            {field.label}
            {field.optional && <span className="text-faint"> · optional</span>}
          </span>
          {field.options ? (
            <select
              value={values[field.name] ?? ''}
              onChange={(e) => onChange({ ...values, [field.name]: e.target.value })}
              className="w-full rounded border border-field-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-ink/40"
            >
              {field.options.map((o) => {
                // A plain string is its own label; { value, label } says
                // something other than what is sent, like "No project" for "".
                const { value, label } = typeof o === 'object' ? o : { value: o, label: o }
                return (
                  <option key={value} value={value}>
                    {label}
                  </option>
                )
              })}
            </select>
          ) : (
          <input
            type={field.type ?? 'text'}
            value={values[field.name] ?? ''}
            placeholder={field.placeholder}
            autoFocus={field.autoFocus}
            onBlur={() => onBlur(field.name)}
            aria-invalid={touched[field.name] && !valid(field, values[field.name]) ? true : undefined}
            onChange={(e) =>
              onChange({
                ...values,
                [field.name]: field.type === 'number' ? Number(e.target.value) : e.target.value,
              })
            }
            className={`w-full rounded border bg-ground px-3 py-2 text-sm outline-none transition focus:border-ink/40 ${
              touched[field.name] && !valid(field, values[field.name]) ? 'border-problem/60' : 'border-field-edge'
            }`}
          />
          )}
          {touched[field.name] && !valid(field, values[field.name]) ? (
            <span className="mt-1 block text-xs text-problem">{field.invalid ?? field.hint}</span>
          ) : (
            field.hint && <span className="mt-1 block text-xs text-muted">{field.hint}</span>
          )}
        </label>
        ),
      )}

      {error && (
        <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
          {error}
        </p>
      )}
    </form>
  )
}

function StepList({ steps }) {
  return (
    <ol className="mt-4 space-y-2">
      {steps.map((step, i) => (
        <li key={i} className="rounded border border-edge bg-ground px-3 py-2">
          <p className="text-xs text-muted">
            {i + 1}. {step.describe}
            {step.optional && <span className="text-faint"> — skipped if it fails</span>}
          </p>
          {step.file ? <FileStep step={step} /> : (
            <pre className="mt-1 overflow-x-auto font-mono text-xs text-ink">{step.argv.join(' ')}</pre>
          )}
        </li>
      ))}
    </ol>
  )
}

// A whole vhost inline pushes everything else off the screen. The file is
// still there in full, one click away — hiding it would be the dishonest fix.
function FileStep({ step }) {
  const [open, setOpen] = useState(false)
  const lines = (step.content ?? '').split('\n').length

  return (
    <>
      <pre className="mt-1 overflow-x-auto font-mono text-xs text-ink">{`cat > ${step.file}`}</pre>
      <button
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="mt-1 font-mono text-xs text-faint transition hover:text-muted"
      >
        {open ? '− hide contents' : `+ show contents (${lines} lines)`}
      </button>
      {open && (
        <pre className="mt-1 max-h-72 overflow-auto rounded border border-edge bg-panel px-2 py-1.5 font-mono text-[11px] leading-relaxed text-muted">
          {step.content}
        </pre>
      )}
    </>
  )
}

export function Progress({ steps, events, error, done }) {
  // How far along, not how much was said. A single step can narrate several
  // times — obtaining a certificate talks to the authority, waits for DNS to
  // propagate, and stores the result, all as step one.
  const current = events.reduce((furthest, e) => Math.max(furthest, e.step ?? 0), 0)
  const entries = group(events)

  // While a step is still running it has not been got through, so counting it
  // would put the bar ahead of the work.
  const through = done || error ? current : Math.max(0, current - 1)

  return (
    <div>
      <div className="flex items-center gap-3">
        <div className="h-1 flex-1 overflow-hidden rounded bg-edge">
          <div
            className={`h-full transition-all ${error ? 'bg-problem' : 'bg-running'}`}
            style={{ width: `${steps.length ? (through / steps.length) * 100 : 0}%` }}
          />
        </div>
        <span className="font-mono text-xs text-muted">
          {through}/{steps.length}
        </span>
      </div>

      {/* Silence and being stuck look identical, and only one of them is
          worth worrying about. */}
      {events.length === 0 && !error && (
        <p className="mt-4 text-xs text-muted">Waiting for the first step to report&hellip;</p>
      )}

      <ol role="log" aria-live="polite" className="mt-4 space-y-2">
        {entries.map((entry, i) => {
          // A step is announced before it runs, so the last one reported is
          // the one happening now — not one that finished. Marking it done
          // was a small lie that made a long step look like a stuck panel.
          const working = !done && !error && i === entries.length - 1

          return (
          <li key={i} className="flex gap-2 text-xs">
            <span className={entry.failed ? 'text-problem' : working ? 'text-caution' : 'text-running'}>
              {entry.failed ? '✕' : working ? <Working /> : '✓'}
            </span>
            <div className="min-w-0 flex-1">
              <p className={entry.failed ? 'text-problem' : working ? 'text-ink' : ''}>
                {entry.text}
                {working && <span className="text-faint"> &mdash; running</span>}
              </p>
              {entry.command && (
                <pre className="overflow-x-auto font-mono text-[11px] text-faint">{entry.command}</pre>
              )}
              {/* What the step said while it worked. Obtaining a certificate
                  talks to the authority and waits for DNS; that is one step
                  narrating, not several steps. */}
              {entry.details.length > 0 && (
                <ul className="mt-1 space-y-0.5 border-l border-edge pl-2.5">
                  {entry.details.map((detail, j) => (
                    <li key={j} className="text-[11px] text-muted">
                      {detail}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </li>
          )
        })}
      </ol>

      <p role="status" className="mt-4 text-xs">
        {done && !error && <span className="text-running">Finished.</span>}
        {error && <span className="text-problem">Stopped: {error}</span>}
      </p>
    </div>
  )
}

// group folds repeated reports for the same step into one entry. A step that
// narrates while it works is still one step, and repeating its command once
// per message suggests it ran that many times.
function group(events) {
  const entries = []

  for (const event of events) {
    const last = entries[entries.length - 1]

    if (last && event.step > 0 && last.step === event.step) {
      last.details.push(event.text)
      if (event.failed) last.failed = true
      continue
    }

    entries.push({
      step: event.step,
      text: event.text,
      command: event.command,
      failed: event.failed,
      details: [],
    })
  }
  return entries
}

// Something that moves, because the whole point is to tell "working" apart
// from "stuck". A tick cannot do that, and for a long install the difference
// is the only thing on screen worth knowing.
function Working() {
  return (
    <span className="relative flex size-2.5 translate-y-[3px] items-center justify-center">
      <span className="absolute inline-flex size-2.5 animate-ping rounded-full bg-caution/60" />
      <span className="relative inline-flex size-1.5 rounded-full bg-caution" />
    </span>
  )
}

// Which methods carry what was filled in. This used to say POST alone, which
// meant editing a route sent nothing at all and the daemon answered "EOF" — a
// word that tells the person nothing about what went wrong or what to do.
function sends(method) {
  return method === 'POST' || method === 'PUT' || method === 'PATCH'
}

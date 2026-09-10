import { useEffect, useRef, useState } from 'react'

// Three states, in order: fill in the form, read the plan, watch it run.
//
// The plan is the dialog — not a "details" disclosure tucked under an OK
// button. Approving a write means approving a list of commands you have read.
export default function PlanDialog({ request, onClose, onFinished }) {
  const [stage, setStage] = useState(request.fields ? 'form' : 'loading')
  const [values, setValues] = useState(request.defaults ?? {})
  const [plan, setPlan] = useState(null)
  const [events, setEvents] = useState([])
  const [error, setError] = useState(null)
  const [done, setDone] = useState(null)
  const stream = useRef(null)

  useEffect(() => {
    if (stage === 'loading') askForPlan(values)
    return () => stream.current?.close()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function askForPlan(body) {
    setStage('loading')
    setError(null)
    try {
      const res = await fetch(`${request.url}${request.url.includes('?') ? '&' : '?'}plan=1`, {
        method: request.method,
        headers: { 'Content-Type': 'application/json' },
        body: request.method === 'POST' ? JSON.stringify(body) : undefined,
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
    setStage('running')
    setEvents([])
    try {
      const res = await fetch(request.url, {
        method: request.method,
        headers: { 'Content-Type': 'application/json' },
        body: request.method === 'POST' ? JSON.stringify(values) : undefined,
      })
      const payload = await res.json()
      if (!res.ok) {
        setError(payload.error ?? `The daemon answered ${res.status}`)
        setStage('error')
        return
      }

      // The work belongs to the daemon. Closing this only stops watching.
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
      source.onerror = () => source.close()
    } catch (e) {
      setError(e.message)
      setStage('error')
    }
  }

  const steps = plan?.plan?.steps ?? []

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-10">
      <div className="w-full max-w-2xl rounded-lg border border-edge bg-panel shadow-2xl">
        <header className="flex items-center justify-between border-b border-edge px-5 py-3">
          <h2 className="text-sm font-medium">{request.title}</h2>
          <button onClick={onClose} className="text-muted hover:text-ink" aria-label="Close">
            ✕
          </button>
        </header>

        <div className="px-5 py-4">
          {stage === 'form' && (
            <Form
              fields={request.fields}
              values={values}
              onChange={setValues}
              error={error}
              onSubmit={() => askForPlan(values)}
            />
          )}

          {stage === 'loading' && <p className="py-6 text-sm text-muted">Working out what this would do…</p>}

          {stage === 'plan' && (
            <>
              <p className="text-sm">{plan.summary}</p>
              <p className="mt-1 text-xs text-muted">
                Nothing has happened yet. These are the commands, in order.
              </p>
              <StepList steps={steps} />
            </>
          )}

          {(stage === 'running' || stage === 'error') && (
            <Progress steps={steps} events={events} error={error} done={done} />
          )}
        </div>

        <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
          <p className="text-xs text-faint">
            {stage === 'plan' && `${steps.length} command${steps.length === 1 ? '' : 's'}`}
          </p>

          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="rounded border border-edge px-3 py-1.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
            >
              {done ? 'Close' : 'Cancel'}
            </button>

            {stage === 'form' && (
              <button
                onClick={() => askForPlan(values)}
                disabled={request.fields?.some((f) => !values[f.name])}
                className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
              >
                Show me the plan
              </button>
            )}

            {stage === 'plan' && (
              <button
                onClick={run}
                className={`rounded border px-3 py-1.5 text-xs transition ${
                  request.destructive
                    ? 'border-problem/50 bg-problem/10 text-problem hover:bg-problem/15'
                    : 'border-edge-strong bg-raised hover:border-ink/30'
                }`}
              >
                Run {steps.length === 1 ? 'this command' : `these ${steps.length} commands`}
              </button>
            )}
          </div>
        </footer>
      </div>
    </div>
  )
}

function Form({ fields, values, onChange, error, onSubmit }) {
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
      className="space-y-3"
    >
      {fields.map((field) => (
        <label key={field.name} className="block">
          <span className="mb-1 block text-xs text-muted">{field.label}</span>
          <input
            type={field.type ?? 'text'}
            value={values[field.name] ?? ''}
            placeholder={field.placeholder}
            autoFocus={field.autoFocus}
            onChange={(e) =>
              onChange({
                ...values,
                [field.name]: field.type === 'number' ? Number(e.target.value) : e.target.value,
              })
            }
            className="w-full rounded border border-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-edge-strong"
          />
          {field.hint && <span className="mt-1 block text-xs text-faint">{field.hint}</span>}
        </label>
      ))}

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
          <pre className="mt-1 overflow-x-auto font-mono text-xs text-ink">
            {step.file ? `cat > ${step.file}` : step.argv.join(' ')}
          </pre>
        </li>
      ))}
    </ol>
  )
}

function Progress({ steps, events, error, done }) {
  // How far along, not how much was said. A single step can narrate several
  // times — obtaining a certificate talks to the authority, waits for DNS to
  // propagate, and stores the result, all as step one.
  const current = events.reduce((furthest, e) => Math.max(furthest, e.step ?? 0), 0)

  return (
    <div>
      <div className="flex items-center gap-3">
        <div className="h-1 flex-1 overflow-hidden rounded bg-edge">
          <div
            className={`h-full transition-all ${error ? 'bg-problem' : 'bg-running'}`}
            style={{ width: `${steps.length ? (current / steps.length) * 100 : 0}%` }}
          />
        </div>
        <span className="font-mono text-xs text-muted">
          {current}/{steps.length}
        </span>
      </div>

      <ol className="mt-4 space-y-1.5">
        {events.map((event, i) => (
          <li key={i} className="flex gap-2 text-xs">
            <span className={event.failed ? 'text-problem' : 'text-running'}>
              {event.failed ? '✕' : '✓'}
            </span>
            <div className="min-w-0">
              <p className={event.failed ? 'text-problem' : ''}>{event.text}</p>
              {event.command && (
                <pre className="overflow-x-auto font-mono text-[11px] text-faint">{event.command}</pre>
              )}
            </div>
          </li>
        ))}
      </ol>

      {done && !error && <p className="mt-4 text-xs text-running">Finished.</p>}
    </div>
  )
}

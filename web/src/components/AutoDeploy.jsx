import { useCallback, useEffect, useId, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import CopyButton from './CopyButton.jsx'
import { ago } from './Activity.jsx'
import { readJSON, humane } from '../lib/api.js'
import { href } from '../lib/useRoute.js'
import { RAISED } from './Container.jsx'

// useAutoDeploy reads whether a service is redeployed on its own when its
// branch moves, and what the last look at the repository found. Read again
// every few seconds while the page is open: a push should be seen arriving.
export function useAutoDeploy(container, service) {
  const url = `/api/hosts/local/instances/${container}/services/${service}/autodeploy`
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [unavailable, setUnavailable] = useState(null)

  const load = useCallback(async () => {
    try {
      setData(await readJSON(await fetch(url)))
      setError(null)
      setUnavailable(null)
    } catch (e) {
      // A panel that cannot deploy on its own says so once; trying again
      // changes nothing.
      if (e.status === 501 || e.status === 404) {
        setUnavailable(e.message)
        return
      }
      setError(humane(e))
    }
  }, [url])

  useEffect(() => {
    setData(null)
    load()
    const timer = setInterval(load, 10000)
    return () => clearInterval(timer)
  }, [load])

  // No host changes, so no plan: the answer is the new state.
  const change = useCallback(
    async (enabled) => {
      const payload = await readJSON(
        await fetch(url, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        }),
      )
      setData(payload)
      setError(null)
      return payload
    },
    [url],
  )

  return { data, error, unavailable, read: data !== null, change, reload: load }
}

// Whether a service follows its branch on its own, and how to make it
// immediate. Turning it on is a button that says what it does; turning it
// off asks first, because it forgets the secret and the repository's webhook
// stops working with it.
export default function AutoDeploy({ service, state, agentDown }) {
  const { data, error, unavailable, read, change } = state
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState(false)
  const [failure, setFailure] = useState(null)
  const [said, setSaid] = useState('')
  const branch = service.branch || 'its branch'
  const minutes = Math.round((data?.everySeconds ?? 180) / 60)

  async function turn(enabled) {
    setBusy(true)
    setFailure(null)
    try {
      await change(enabled)
      setAsking(false)
      setSaid(enabled ? 'Automatic deployments are on.' : 'Automatic deployments are off.')
    } catch (e) {
      setFailure(humane(e))
    } finally {
      setBusy(false)
    }
  }

  const on = Boolean(data?.enabled)

  return (
    <Card
      title="Automatic deployments"
      action={read && <Chip label={on ? 'on' : 'off'} tone={on ? 'border-ink/40 text-ink' : 'border-edge text-muted'} />}
    >
      <p role="status" className="sr-only">{said}</p>

      {unavailable ? (
        <p className="px-4 py-6 text-center text-xs text-muted">
          This panel cannot deploy on its own. The daemon said: {unavailable}.
        </p>
      ) : !read ? (
        error ? (
          <p role="alert" className="px-4 py-6 text-center text-xs text-problem">
            Could not read whether {service.name} deploys on its own. {error}
          </p>
        ) : (
          <p role="status" className="px-4 py-6 text-center text-xs text-muted">Reading&hellip;</p>
        )
      ) : (
        <>
          <div className="space-y-3 px-4 py-4">
            <p className="text-sm">
              {on ? (
                <>
                  Croft redeploys <span className="font-mono">{service.name}</span> whenever{' '}
                  <span className="font-mono">{branch}</span> gets a new commit.
                </>
              ) : (
                <>
                  Redeploy <span className="font-mono">{service.name}</span> whenever{' '}
                  <span className="font-mono">{branch}</span> gets a new commit.
                </>
              )}
            </p>
            <ul className="list-disc space-y-1 pl-4 text-xs text-muted">
              <li>
                With the repository&apos;s webhook set up, it starts at once. Without it, croft checks the
                repository every {minutes} minutes.
              </li>
              <li>A commit that fails to deploy is not tried again; the next commit is.</li>
              <li>
                Each one is a redeploy like any other — snapshot first, then the plan — listed in{' '}
                <a href={href('activity')} className="underline underline-offset-2 hover:text-ink">Activity</a>{' '}
                as &ldquo;Automatic deployment&rdquo;.
              </li>
            </ul>

            {on && <LastCheck data={data} />}
          </div>

          {on && <Webhook service={service} data={data} />}

          {(failure || (error && read)) && (
            <p role="alert" className="border-t border-edge px-4 py-2.5 text-xs text-problem">
              {failure ?? `Could not read it again just now: ${error}`}
            </p>
          )}

          <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-edge px-4 py-3">
            {asking ? (
              <TurnOff onKeep={() => setAsking(false)} onTurnOff={() => turn(false)} busy={busy} />
            ) : on ? (
              <>
                <p className="text-xs text-muted">Turning it off makes the webhook stop working.</p>
                <button
                  onClick={() => setAsking(true)}
                  disabled={busy}
                  className="rounded border border-edge px-2.5 py-1 text-xs text-muted transition hover:border-problem/50 hover:text-problem disabled:opacity-40"
                >
                  Turn off&hellip;
                </button>
              </>
            ) : (
              <>
                <p className="text-xs text-muted">
                  {agentDown ? 'The agent is not answering, so this cannot be changed now.' : 'Nothing is deployed by turning it on; the next new commit is.'}
                </p>
                <button
                  onClick={() => turn(true)}
                  disabled={busy || agentDown}
                  className={`${RAISED} disabled:opacity-40`}
                >
                  {busy ? 'Turning on…' : 'Turn on automatic deployments'}
                </button>
              </>
            )}
          </footer>
        </>
      )}
    </Card>
  )
}

function TurnOff({ onKeep, onTurnOff, busy }) {
  return (
    <div role="alertdialog" aria-label="Turn off automatic deployments?" className="flex w-full flex-wrap items-center justify-between gap-3">
      <p className="text-xs text-ink">
        Turn off? Croft forgets the secret, so the repository&apos;s webhook stops working. Turning it on
        again makes a new secret to paste there.
      </p>
      <div className="flex shrink-0 gap-2">
        <button
          autoFocus
          onClick={onKeep}
          className="rounded border border-edge-strong bg-raised px-3 py-1 text-xs transition hover:border-ink/30"
        >
          Keep it on
        </button>
        <button
          onClick={onTurnOff}
          disabled={busy}
          className="rounded border border-problem/50 px-3 py-1 text-xs text-problem transition hover:bg-problem/10 disabled:opacity-40"
        >
          {busy ? 'Turning off…' : 'Turn off'}
        </button>
      </div>
    </div>
  )
}

// What the last look at the repository found, and the commit that failed
// when one did — said beside each other, since the second explains why the
// first says nothing is happening.
function LastCheck({ data }) {
  const last = data.last
  return (
    <div className="space-y-2">
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 rounded border border-edge bg-ground/40 px-3 py-2 text-xs">
        <dt className="text-muted">Last check</dt>
        <dd>
          {last ? (
            <>
              {ago(new Date(last.at))}
              <span className="text-muted"> · {last.by === 'webhook' ? 'from the webhook' : 'periodic check'}</span>
            </>
          ) : (
            <span className="text-muted">none yet — the first is within {Math.round((data.everySeconds ?? 180) / 60)} minutes</span>
          )}
        </dd>
        {last && (
          <>
            <dt className="text-muted">Result</dt>
            <dd className={last.error ? 'text-problem' : ''}>
              {last.result}
              {last.tip && <span className="font-mono text-muted"> · {last.tip.slice(0, 7)}</span>}
            </dd>
          </>
        )}
        {last?.error && (
          <>
            <dt className="text-muted">Error</dt>
            <dd className="break-words font-mono text-[11px] text-problem">{last.error}</dd>
          </>
        )}
      </dl>
      {data.failed && (
        <p className="rounded border border-caution/30 px-3 py-2 text-xs text-caution">
          Commit <span className="font-mono">{data.failed.slice(0, 7)}</span> failed to deploy, and is not tried
          again. Push a fix and the next commit deploys — the steps it stopped at are in{' '}
          <a href={href('activity')} className="underline underline-offset-2">Activity</a>.
        </p>
      )}
    </div>
  )
}

// Where the repository is told to call, and the secret it signs with. Which
// host it is — GitHub or GitLab — is read off the repository's address, so
// the steps shown first are the ones that apply.
function Webhook({ service, data }) {
  const [revealed, setRevealed] = useState(false)
  const guess = /gitlab/i.test(service.repo ?? '') ? 'gitlab' : 'github'
  const [host, setHost] = useState(guess)
  const url = `${window.location.origin}${data.hook}`
  const secretId = useId()
  // A webhook comes from the internet: an address only this network can
  // reach is one it never arrives at. The periodic check still works.
  const local = /^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|\[?::1)/.test(window.location.hostname)

  return (
    <section aria-labelledby={`${secretId}-title`} className="border-t border-edge px-4 py-4">
      <h3 id={`${secretId}-title`} className="text-xs font-medium">
        Make it instant <span className="font-normal text-muted">· optional</span>
      </h3>
      <p className="mt-0.5 text-xs text-muted">Add a webhook to the repository with these two values.</p>

      <div className="mt-3 grid gap-3 md:grid-cols-2">
        <Value label="Webhook URL" value={url} />
        <div className="min-w-0">
          <p id={secretId} className="mb-1 text-xs text-muted">Secret</p>
          <div className="flex items-center gap-1.5">
            <code className="flex h-8 min-w-0 flex-1 items-center overflow-hidden rounded border border-edge bg-ground px-2.5 font-mono text-xs">
              {revealed ? (
                <span className="truncate">{data.secret}</span>
              ) : (
                <>
                  <span aria-hidden="true" className="truncate text-muted">{'•'.repeat(24)}</span>
                  <span className="sr-only">hidden</span>
                </>
              )}
            </code>
            <button
              type="button"
              onClick={() => setRevealed(!revealed)}
              aria-label={revealed ? 'Hide the secret' : 'Show the secret'}
              className="inline-flex min-h-6 items-center rounded border border-edge bg-panel px-1.5 py-0.5 text-[11px] text-muted transition hover:text-ink"
            >
              {revealed ? 'hide' : 'show'}
            </button>
            <CopyButton text={data.secret ?? ''} label="Copy the secret" />
          </div>
        </div>
      </div>

      {local && (
        <p className="mt-3 text-xs text-caution">
          This panel is open at {window.location.hostname}, which {host === 'gitlab' ? 'GitLab' : 'GitHub'} cannot
          reach. Serve the panel at a public domain (in{' '}
          <a href={href('settings')} className="underline underline-offset-2">Settings</a>) for the webhook to
          arrive; until then the check every few minutes still deploys.
        </p>
      )}

      <div className="mt-4">
        <div role="group" aria-label="Where the repository is" className="flex gap-1">
          {[
            ['github', 'GitHub'],
            ['gitlab', 'GitLab'],
          ].map(([id, label]) => (
            <button
              key={id}
              type="button"
              aria-pressed={host === id}
              onClick={() => setHost(id)}
              className={`rounded border px-2 py-0.5 text-xs transition ${
                host === id ? 'border-edge-strong bg-raised text-ink' : 'border-transparent text-muted hover:text-ink'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        {host === 'github' ? (
          <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-muted">
            <li>In the repository, open <b className="font-medium text-ink">Settings → Webhooks → Add webhook</b>.</li>
            <li><b className="font-medium text-ink">Payload URL</b>: the webhook URL above.</li>
            <li><b className="font-medium text-ink">Content type</b>: <span className="font-mono">application/json</span>.</li>
            <li><b className="font-medium text-ink">Secret</b>: the secret above.</li>
            <li>Choose <b className="font-medium text-ink">Just the push event</b>, then <b className="font-medium text-ink">Add webhook</b>.</li>
          </ol>
        ) : (
          <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-muted">
            <li>In the project, open <b className="font-medium text-ink">Settings → Webhooks → Add new webhook</b>.</li>
            <li><b className="font-medium text-ink">URL</b>: the webhook URL above.</li>
            <li><b className="font-medium text-ink">Secret token</b>: the secret above.</li>
            <li>Under <b className="font-medium text-ink">Trigger</b>, tick <b className="font-medium text-ink">Push events</b>, then <b className="font-medium text-ink">Add webhook</b>.</li>
          </ol>
        )}
      </div>
    </section>
  )
}

// A value to paste somewhere else: read-only, in full on hover and to a
// screen reader, and one press from the clipboard.
export function Value({ label, value }) {
  const id = useId()
  return (
    <div className="min-w-0">
      <p id={id} className="mb-1 text-xs text-muted">{label}</p>
      <div className="flex items-center gap-1.5">
        <code
          title={value}
          className="flex h-8 min-w-0 flex-1 items-center overflow-hidden rounded border border-edge bg-ground px-2.5 font-mono text-xs"
        >
          <span className="truncate">{value}</span>
        </code>
        <CopyButton text={value} label={`Copy the ${label.toLowerCase()}`} />
      </div>
    </div>
  )
}

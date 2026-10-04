import { useState } from 'react'
import Command from './Command.jsx'
import CopyButton from './CopyButton.jsx'
import { INTERVAL } from '../lib/useOverview.js'

// The panel answers, the agent behind it does not. It is said once, in one
// place, with what to type on the server — not as a generic failure that
// sends somebody to restart the panel, which is already running.

export const AGENT_BLOCKED =
  'The croft agent is not answering, so nothing can be changed right now. This reconnects by itself.'

const FIX = ['systemctl status croft-agent', '# if it is stopped:', 'sudo systemctl restart croft-agent']

const RECONNECTS = `This page reconnects by itself — checking every ${Math.round(INTERVAL / 1000)} seconds.`

// The first public address, or the first of any: where to ssh to.
export function primaryAddress(host) {
  const addresses = host?.addresses ?? []
  return addresses.find((a) => a.public) ?? addresses[0] ?? null
}

// Check now waits for its own answer only. The ten-second poll does not
// touch it, so the button does not flicker to "Checking…" on its own.
function CheckNow({ onCheck }) {
  const [checking, setChecking] = useState(false)

  async function check() {
    setChecking(true)
    try {
      await onCheck()
    } finally {
      setChecking(false)
    }
  }

  return (
    <button
      type="button"
      onClick={check}
      disabled={checking}
      aria-busy={checking}
      className="min-h-6 rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30 disabled:cursor-wait disabled:text-muted"
    >
      {checking ? 'Checking…' : 'Check now'}
    </button>
  )
}

// Nothing has been read yet, so there is nothing to show behind it.
export function AgentDownScreen({ detail, host, onCheck }) {
  const address = primaryAddress(host)

  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-8 sm:px-6">
      <div role="alert" className="w-full max-w-lg space-y-3 rounded-lg border border-problem/30 bg-panel px-5 py-4">
        <h1 className="text-sm font-medium text-problem">The croft agent is not answering.</h1>
        <p className="text-xs text-muted">
          The panel is up, but the agent that reads and changes this server is not, so there is nothing to show
          and nothing can be changed until it is back.
        </p>

        {address && (
          <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
            <span>On the server,</span>
            <span className="font-mono text-ink">{address.address}</span>
            {!address.public && <span className="text-faint">private</span>}
            <CopyButton text={address.address} label={`Copy ${address.address}`} />
          </p>
        )}

        <Command lines={FIX} />

        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted">{RECONNECTS}</p>
          <CheckNow onCheck={onCheck} />
        </div>

        {detail && (
          <details className="text-xs">
            <summary className="cursor-pointer text-muted transition hover:text-ink">What the panel saw</summary>
            <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all rounded border border-edge bg-ground px-3 py-2 font-mono text-faint">
              {detail}
            </pre>
          </details>
        )}
      </div>
    </div>
  )
}

// The last reading is still worth seeing; the banner says how old it is and
// that nothing on it can be changed. Its text does not move between polls:
// a sentence that repaints every ten seconds is read again by nobody.
export function AgentBanner({ fetchedAt, onCheck }) {
  return (
    <div className="space-y-2.5 rounded-lg border border-problem/30 bg-panel px-4 py-3 text-xs">
      <p role="alert">
        <span className="text-problem">The croft agent is not answering.</span>{' '}
        <span className="text-muted">
          Showing the last reading{fetchedAt ? ` from ${fetchedAt.toLocaleTimeString()}` : ''}; nothing can be
          changed until it is back.
        </span>
      </p>
      <Command lines={FIX} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted">{RECONNECTS}</p>
        <CheckNow onCheck={onCheck} />
      </div>
    </div>
  )
}

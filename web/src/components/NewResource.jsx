import { useState } from 'react'
import { Frame } from './DeployDialog.jsx'
import { PRIMARY, SECONDARY } from '../lib/ui.js'

// Making something used to start from three places — a container from Home,
// a service or a database from inside the container, adoption from what the
// container found. This asks the two questions in the order a person has
// them: what, then where. Each answer leads to the dialog that already does
// it; nothing here is a second way of making anything.
const WHAT = [
  { id: 'service', label: 'Service', detail: 'Code from a repository, built and kept running.' },
  { id: 'database', label: 'Database', detail: 'An engine inside the container, so its snapshots hold the data too.' },
  { id: 'adopt', label: 'Something already running', detail: 'A unit or site croft found in a container, taken on as it is.' },
]

export default function NewResource({ project, containers, onClose, onService, onDatabase, onAdopt, onNewContainer }) {
  const running = containers.filter((c) => c.status === 'running')
  const [what, setWhat] = useState('service')
  const [where, setWhere] = useState(running[0]?.name ?? '')

  const go = () => {
    if (where === '') return onNewContainer()
    const container = containers.find((c) => c.name === where)
    if (what === 'service') onService(container)
    if (what === 'database') onDatabase(container)
    if (what === 'adopt') onAdopt(container)
  }

  return (
    <Frame title={project ? `New resource in ${project}` : 'New resource'} onClose={onClose}>
      <div className="space-y-5 px-5 py-4">
        <fieldset className="space-y-2">
          <legend className="mb-2 text-xs uppercase tracking-wide text-faint">What</legend>
          {WHAT.map((w) => (
            <label key={w.id} className={`flex cursor-pointer gap-3 rounded-md border px-3 py-2.5 transition ${what === w.id ? 'border-action/60 bg-action/[0.06]' : 'border-edge hover:border-edge-strong'}`}>
              <input type="radio" name="what" value={w.id} checked={what === w.id} onChange={() => { setWhat(w.id); if (w.id === 'adopt' && where === '') setWhere(running[0]?.name ?? '') }} className="mt-1 accent-[#8ab4f8]" />
              <span>
                <span className="block text-sm">{w.label}</span>
                <span className="block text-xs text-muted">{w.detail}</span>
              </span>
            </label>
          ))}
        </fieldset>

        <fieldset className="space-y-2">
          <legend className="mb-2 text-xs uppercase tracking-wide text-faint">Where</legend>
          {containers.map((c) => {
            const off = c.status !== 'running'
            return (
              <label key={c.name} className={`flex gap-3 rounded-md border px-3 py-2.5 transition ${off ? 'cursor-not-allowed border-edge opacity-60' : where === c.name ? 'cursor-pointer border-action/60 bg-action/[0.06]' : 'cursor-pointer border-edge hover:border-edge-strong'}`}>
                <input type="radio" name="where" value={c.name} disabled={off} checked={where === c.name} onChange={() => setWhere(c.name)} className="mt-1 accent-[#8ab4f8]" />
                <span>
                  <span className="block font-mono text-sm">{c.name}</span>
                  <span className="block text-xs text-muted">
                    {off ? 'Stopped. Start it first: nothing can be deployed into a stopped container.' : c.address || 'running'}
                  </span>
                </span>
              </label>
            )
          })}
          {what !== 'adopt' && (
            <label className={`flex cursor-pointer gap-3 rounded-md border px-3 py-2.5 transition ${where === '' ? 'border-action/60 bg-action/[0.06]' : 'border-edge hover:border-edge-strong'}`}>
              <input type="radio" name="where" value="" checked={where === ''} onChange={() => setWhere('')} className="mt-1 accent-[#8ab4f8]" />
              <span>
                <span className="block text-sm">A new container</span>
                <span className="block text-xs text-muted">Made first{project ? ` in ${project}` : ''}; then deploy into it from its page.</span>
              </span>
            </label>
          )}
        </fieldset>
      </div>
      <footer className="flex justify-end gap-2 border-t border-edge px-5 py-3">
        <button onClick={onClose} className={SECONDARY}>Cancel</button>
        <button onClick={go} disabled={what === 'adopt' && where === ''} className={PRIMARY}>Continue</button>
      </footer>
    </Frame>
  )
}

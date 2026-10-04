import { useEffect, useState } from 'react'
import PlanDialog, { Discard } from './PlanDialog.jsx'
import EnvEditor, { fromObject, toObject, invalidKeys } from './EnvEditor.jsx'
import { Frame } from './DeployDialog.jsx'
import { readJSON, humane } from '../lib/api.js'

// A service's environment, read from the file it lives in and written back to
// it. The file is the only copy: an edit made over ssh is what this shows the
// next time it opens, and a save is refused if the file changed since it was
// opened here — so nobody's edit is quietly put back.
export default function EnvDialog({ container, service, peers, onClose, onFinished }) {
  const [found, setFound] = useState(null)
  const [text, setText] = useState('')
  const [original, setOriginal] = useState('')
  const [error, setError] = useState(null)
  const [saving, setSaving] = useState(false)
  const [reads, setReads] = useState(0)
  // Reading the file again replaces whatever was typed here, so with changes
  // in the editor it asks first — the same question closing asks.
  const [asking, setAsking] = useState(false)

  const url = `/api/hosts/local/instances/${container.name}/services/${service.name}/env`

  useEffect(() => {
    let current = true
    setFound(null)
    setError(null)
    fetch(url)
      .then(readJSON)
      .then((payload) => {
        if (!current) return
        setFound(payload)
        const read = fromObject(Object.fromEntries((payload.vars ?? []).map((v) => [v.key, v.value])))
        setOriginal(read)
        setText(read)
      })
      .catch((e) => current && setError(humane(e)))
    return () => {
      current = false
    }
  }, [url, reads])

  if (saving) {
    return (
      <PlanDialog
        request={{
          title: `Environment of ${service.name}`,
          url,
          method: 'POST',
          defaults: { hash: found.hash, vars: toObject(text) },
        }}
        onClose={onClose}
        dirty
        onBack={() => setSaving(false)}
        onFinished={onFinished}
      />
    )
  }

  const rebuild = found?.applies === 'rebuild'
  const reading = !found && !error
  const dirty = JSON.stringify(toObject(text)) !== JSON.stringify(toObject(original))
  const invalid = invalidKeys(text)

  function readAgain() {
    setAsking(false)
    setReads((n) => n + 1)
  }

  return (
    <Frame title={`Environment of ${service.name}`} onClose={onClose} dirty={dirty}>
      <div className="space-y-3 px-5 py-4">
        {error && (
          <p role="alert" className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
            {error}
          </p>
        )}
        {reading && <p role="status" className="text-xs text-muted">Reading the file&hellip;</p>}

        {found && (
          <>
            <p className="text-xs text-muted">
              <span className="font-mono">{found.file}</span>
              {found.exists ? '' : ' — not there yet; saving creates it, readable only by its owner'}
            </p>
            <EnvEditor
              value={text}
              onChange={setText}
              peers={peers}
              note={
                rebuild
                  ? 'Read by the build. Saving rewrites only what you changed, then rebuilds and publishes the site.'
                  : 'Read by the unit when it starts. Saving rewrites only what you changed — comments and the rest stay — then restarts it. If this service reads them when it is built instead, Redeploy after saving.'
              }
            />
          </>
        )}
      </div>

      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-edge px-5 py-3">
        {asking ? (
          <Discard onKeep={() => setAsking(false)} onDiscard={readAgain} verb="Discard and read again" />
        ) : (
          <>
            <button
              onClick={() => (dirty ? setAsking(true) : readAgain())}
              disabled={reading}
              className="text-xs text-muted transition hover:text-ink disabled:opacity-40"
            >
              {reading ? 'Reading…' : 'Read the file again'}
            </button>
            <div className="flex items-center gap-3">
              {invalid.length > 0 && (
                <span className="text-xs text-muted">Fix the names marked above first.</span>
              )}
              <button
                onClick={() => setSaving(true)}
                // Nothing changed is nothing to plan.
                disabled={!found || !dirty || invalid.length > 0}
                className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
              >
                Show me the plan
              </button>
            </div>
          </>
        )}
      </footer>
    </Frame>
  )
}

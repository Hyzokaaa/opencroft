import { useState } from 'react'
import PlanDialog from './PlanDialog.jsx'

// Deploying is two plans, not one, because you cannot know how to build code
// you have not seen. First: "I am going to look at the repository" — the git
// commands, in plain sight. Then: "this is what I found, this is what I would
// do" — every command editable before any of it runs.
//
// That is the whole difference from a buildpack. We guess as much as anyone
// does; we just do it where you can see it and change it.
export default function DeployDialog({ container, onClose, onFinished }) {
  const [stage, setStage] = useState('source')
  const [source, setSource] = useState({ repo: '', branch: 'main', path: '/srv/app' })
  const [app, setApp] = useState(null)
  const [why, setWhy] = useState('')
  const [error, setError] = useState(null)

  // The inspect plan is approved through the very same dialog every other
  // write uses. Nothing about this flow gets a shortcut past the plan.
  if (stage === 'inspecting') {
    return (
      <PlanDialog
        request={{
          title: `Look at ${source.repo}`,
          url: `/api/hosts/local/instances/${container.name}/inspect`,
          method: 'POST',
          defaults: source,
          // The answer is a detection, not a job, so the dialog hands it back
          // instead of watching a stream.
          immediate: true,
          working: 'Cloning and reading the repository…',
        }}
        onClose={onClose}
        onResult={(detection) => {
          setWhy(detection.why ?? '')
          setApp({
            install: (detection.install ?? []).join(' && '),
            build: (detection.build ?? []).join(' && '),
            start: detection.start ?? '',
            runtime: detection.runtime ?? '',
            port: detection.port ?? 0,
            packages: (detection.packages ?? []).join(' '),
          })
          setStage('found')
        }}
      />
    )
  }

  if (stage === 'deploying') {
    return (
      <PlanDialog
        request={{
          title: `Deploy to ${container.name}`,
          url: `/api/hosts/local/instances/${container.name}/deploy`,
          method: 'POST',
          defaults: {
            ...source,
            install: split(app.install),
            build: split(app.build),
            start: app.start.trim(),
            runtime: app.runtime,
            port: Number(app.port) || 0,
            packages: app.packages.split(/\s+/).filter(Boolean),
          },
        }}
        onClose={onClose}
        onFinished={onFinished}
      />
    )
  }

  return (
    <Frame title={`Deploy a project to ${container.name}`} onClose={onClose}>
      {stage === 'source' && (
        <Source
          value={source}
          onChange={setSource}
          error={error}
          onSubmit={() => {
            if (!source.repo.trim()) {
              setError('A repository URL is required.')
              return
            }
            setError(null)
            setStage('inspecting')
          }}
        />
      )}

      {stage === 'found' && (
        <Found app={app} why={why} onChange={setApp} onDeploy={() => setStage('deploying')} />
      )}
    </Frame>
  )
}

function split(joined) {
  return joined.split('&&').map((c) => c.trim()).filter(Boolean)
}

function Frame({ title, onClose, children }) {
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-10">
      <div className="w-full max-w-2xl rounded-lg border border-edge bg-panel shadow-2xl">
        <header className="flex items-center justify-between border-b border-edge px-5 py-3">
          <h2 className="text-sm font-medium">{title}</h2>
          <button onClick={onClose} className="text-muted hover:text-ink" aria-label="Close">
            ✕
          </button>
        </header>
        {children}
      </div>
    </div>
  )
}

function Source({ value, onChange, error, onSubmit }) {
  return (
    <>
      <form onSubmit={(e) => { e.preventDefault(); onSubmit() }} className="space-y-3 px-5 py-4">
        <Field
          label="Repository"
          hint="A public https URL. Private repositories need a deploy key, which is not built yet."
          autoFocus
          placeholder="https://github.com/you/app.git"
          value={value.repo}
          onChange={(repo) => onChange({ ...value, repo })}
        />

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Branch" value={value.branch} onChange={(branch) => onChange({ ...value, branch })} />
          <Field
            label="Where it lands in the container"
            value={value.path}
            onChange={(path) => onChange({ ...value, path })}
          />
        </div>

        {error && (
          <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
            {error}
          </p>
        )}
      </form>

      <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
        <p className="text-xs text-faint">Nothing is built yet — only fetched, so it can be read.</p>
        <button
          onClick={onSubmit}
          className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30"
        >
          Inspect the repository
        </button>
      </footer>
    </>
  )
}

// Everything here is a text box on purpose. What was detected is a suggestion,
// and a suggestion you cannot edit is a decision made behind your back.
function Found({ app, why, onChange, onDeploy }) {
  const set = (key) => (v) => onChange({ ...app, [key]: v })

  return (
    <>
      <div className="space-y-3 px-5 py-4">
        <p className="text-sm">
          {app.runtime ? `Detected ${app.runtime}.` : 'Nothing recognisable at the root.'}{' '}
          <span className="text-muted">{why}</span>
        </p>
        <p className="text-xs text-faint">
          These are suggestions, not rules. Change anything &mdash; what you leave here is what runs,
          and what gets written onto the container.
        </p>

        <Field label="Install" value={app.install} onChange={set('install')} mono
          hint="Run in the checkout. Several commands joined with &&." />
        <Field label="Build" value={app.build} onChange={set('build')} mono
          hint="Left empty, no build step happens." />
        <Field label="Start" value={app.start} onChange={set('start')} mono
          hint="What systemd runs, and restarts if it exits." />

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Port it listens on" type="number" value={app.port} onChange={set('port')} />
          <Field label="Packages to install first" value={app.packages} onChange={set('packages')} mono
            hint="git is always installed." />
        </div>
      </div>

      <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
        <p className="text-xs text-faint">
          {app.start?.trim()
            ? 'A snapshot is taken first, so this can be undone.'
            : 'Nothing says how to start it, so there is no deployment to run.'}
        </p>
        <button
          onClick={onDeploy}
          disabled={!app.start?.trim()}
          className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
        >
          Show me the deploy plan
        </button>
      </footer>
    </>
  )
}

function Field({ label, hint, value, onChange, mono, type, placeholder, autoFocus }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-muted">{label}</span>
      <input
        type={type ?? 'text'}
        value={value ?? ''}
        placeholder={placeholder}
        autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value)}
        className={`w-full rounded border border-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-edge-strong ${
          mono ? 'font-mono text-xs' : ''
        }`}
      />
      {hint && <span className="mt-1 block text-xs text-faint">{hint}</span>}
    </label>
  )
}

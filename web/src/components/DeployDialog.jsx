import { useState } from 'react'
import PlanDialog from './PlanDialog.jsx'
import EnvEditor, { toObject, fromObject } from './EnvEditor.jsx'

// Deploying is two plans, not one, because you cannot know how to build code
// you have not seen. First: "I am going to look at the repository" — the git
// commands, in plain sight. Then: "this is what I found, this is what I would
// do" — every command editable before any of it runs.
//
// That is the whole difference from a buildpack. We guess as much as anyone
// does; we just do it where you can see it and change it.
export default function DeployDialog({ container, service: deployed, onClose, onFinished }) {
  // Deploying something already here is the same second half, with what the
  // container remembers instead of what was just detected. Asking again for a
  // repository it already knows would be asking a question we can answer.
  const [stage, setStage] = useState(deployed ? 'found' : 'source')
  const [source, setSource] = useState(() => ({
    repo: deployed?.repo ?? '',
    branch: deployed?.branch ?? 'main',
    name: deployed?.name ?? '',
    path: deployed?.path ?? '',
  }))
  const [service, setService] = useState(() => (deployed ? remembered(deployed) : null))
  const [why, setWhy] = useState(
    deployed ? 'What this container remembers. Change anything before it runs again.' : '',
  )
  const [error, setError] = useState(null)

  const name = source.name || guessName(source.repo)
  const asked = { ...source, name }

  if (stage === 'inspecting') {
    return (
      <PlanDialog
        request={{
          title: `Look at ${source.repo}`,
          url: `/api/hosts/local/instances/${container.name}/services/inspect`,
          method: 'POST',
          defaults: asked,
          // The answer is a detection, not a job, so the dialog hands it back
          // instead of watching a stream.
          immediate: true,
          working: 'Cloning and reading the repository…',
        }}
        onClose={onClose}
        onResult={(detection) => {
          setWhy(detection.why ?? '')
          setService({
            ...asked,
            install: (detection.install ?? []).join(' && '),
            build: (detection.build ?? []).join(' && '),
            start: detection.start ?? '',
            runtime: detection.runtime ?? '',
            port: detection.port ?? 0,
            packages: (detection.packages ?? []).join(' '),
            env: '',
            health: '',
            contains: '',
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
          title: `Deploy ${service.name} to ${container.name}`,
          url: `/api/hosts/local/instances/${container.name}/services/deploy`,
          method: 'POST',
          defaults: {
            name: service.name,
            repo: service.repo,
            branch: service.branch,
            path: service.path,
            install: split(service.install),
            build: split(service.build),
            start: service.start.trim(),
            runtime: service.runtime,
            port: Number(service.port) || 0,
            packages: service.packages.split(/\s+/).filter(Boolean),
            env: toObject(service.env),
            health: { path: service.health.trim(), contains: service.contains.trim(), status: 0 },
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
          name={name}
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
        <Found service={service} why={why} onChange={setService} onDeploy={() => setStage('deploying')} />
      )}
    </Frame>
  )
}

// remembered turns what is stored on the container back into what the form
// edits. The environment comes back too — masked in the editor, but present,
// so changing one variable does not mean retyping the other nineteen.
function remembered(deployed) {
  return {
    repo: deployed.repo,
    branch: deployed.branch,
    name: deployed.name,
    path: deployed.path,
    install: (deployed.install ?? []).join(' && '),
    build: (deployed.build ?? []).join(' && '),
    start: deployed.start ?? '',
    runtime: deployed.runtime ?? '',
    port: deployed.port ?? 0,
    packages: (deployed.packages ?? []).join(' '),
    env: fromObject(deployed.env),
    health: deployed.health?.path ?? '',
    contains: deployed.health?.contains ?? '',
  }
}

// The repository name is what anybody would have typed anyway.
function guessName(repo) {
  const last = (repo ?? '').replace(/\/+$/, '').split('/').pop() ?? ''
  return last.replace(/\.git$/, '').toLowerCase()
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

function Source({ value, name, onChange, error, onSubmit }) {
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

        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Branch" value={value.branch} onChange={(branch) => onChange({ ...value, branch })} />
          <Field
            label="Call it"
            placeholder={name || 'app'}
            value={value.name}
            onChange={(n) => onChange({ ...value, name: n })}
            hint="Names its unit and its snapshots."
          />
          <Field
            label="Where it lands"
            placeholder={name ? `/srv/${name}` : '/srv/app'}
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

// Everything here is editable on purpose. What was detected is a suggestion,
// and a suggestion you cannot change is a decision made behind your back.
function Found({ service, why, onChange, onDeploy }) {
  const set = (key) => (v) => onChange({ ...service, [key]: v })

  return (
    <>
      <div className="space-y-3 px-5 py-4">
        <p className="text-sm">
          {service.runtime ? `Detected ${service.runtime}.` : 'Nothing recognisable at the root.'}{' '}
          <span className="text-muted">{why}</span>
        </p>
        <p className="text-xs text-faint">
          These are suggestions, not rules. Change anything &mdash; what you leave here is what runs,
          and what gets written onto the container.
        </p>

        <Field label="Install — has to finish" value={service.install} onChange={set("install")} mono
          hint="Run in the checkout. Several commands joined with &&." />
        <Field label="Build — has to finish" value={service.build} onChange={set("build")} mono
          hint="Left empty, no build step happens." />
        <Field label="Start — keeps running" value={service.start} onChange={set("start")} mono
          hint="What systemd runs, and restarts if it exits. The server goes here, not above." />

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Port it listens on" type="number" value={service.port} onChange={set('port')} />
          <Field label="Packages to install first" value={service.packages} onChange={set('packages')} mono
            hint="git and curl are always installed." />
        </div>

        <EnvEditor value={service.env} onChange={set('env')} />

        {/* Nothing reports readiness, so without somewhere to ask, a
            deployment is finished when the unit is up — which is a weaker
            promise, and the panel says so rather than implying more. */}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Ready when this answers" value={service.health} onChange={set('health')} mono
            placeholder="/health"
            hint={service.health.trim()
              ? 'Asked from the host, for up to 30 seconds.'
              : 'Left empty, the deployment ends when the unit starts.'} />
          <Field label="…and the body contains" value={service.contains} onChange={set('contains')} mono
            placeholder="optional" />
        </div>
      </div>

      <footer className="flex items-center justify-between gap-3 border-t border-edge px-5 py-3">
        <p className="text-xs text-faint">
          {service.start?.trim()
            ? 'A snapshot is taken first, so this can be undone.'
            : 'Nothing says how to start it, so there is no deployment to run.'}
        </p>
        <button
          onClick={onDeploy}
          disabled={!service.start?.trim()}
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

import { useState } from 'react'
import { useDialog, dialogProps } from '../lib/useDialog.js'
import { Discard } from './PlanDialog.jsx'
import PlanDialog from './PlanDialog.jsx'
import EnvEditor, { toObject, invalidKeys } from './EnvEditor.jsx'

// Deploying is two plans, not one, because you cannot know how to build code
// you have not seen. First: "I am going to look at the repository" — the git
// commands, in plain sight. Then: "this is what I found, this is what I would
// do" — every command editable before any of it runs.
//
// That is the whole difference from a buildpack. We guess as much as anyone
// does; we just do it where you can see it and change it.
export default function DeployDialog({ container, service: deployed, peers, onClose, onFinished, onAddDomain }) {
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
  // What was saved when this opened, to tell a change from none.
  const [original] = useState(() => (deployed ? remembered(deployed) : null))
  const [why, setWhy] = useState(
    deployed ? `Saved on the container. Changes take effect the next time ${deployed.name} is deployed.` : '',
  )
  const changed = Boolean(service) && (!original || JSON.stringify(body(service)) !== JSON.stringify(body(original)))
  // Anything worth asking about before it is thrown away: a new service is
  // all unsaved work once a repository has been typed; an existing one only
  // when something was changed.
  const sourceChanged = Boolean(deployed) && (source.repo !== (deployed.repo ?? '') || source.branch !== (deployed.branch ?? 'main') || source.path !== (deployed.path ?? ''))
  const dirty = deployed ? changed || sourceChanged : Boolean(source.repo.trim()) || Boolean(service)
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
        dirty={dirty}
        onBack={() => setStage('source')}
        onResult={(detection) => {
          setWhy(detection.why ?? '')
          // Re-detecting a service that already exists — after changing its
          // branch, say — looks at the new code, not at what it is configured
          // to do with it. The environment and the readiness check are not on
          // the branch; wiping them here would make "change the branch" also
          // mean "forget the secrets".
          setService((prev) => ({
            ...asked,
            install: (detection.install ?? []).join(' && '),
            build: (detection.build ?? []).join(' && '),
            start: detection.start ?? '',
            runtime: detection.runtime ?? '',
            port: detection.port ?? 0,
            packages: (detection.packages ?? []).join(' '),
            env: prev?.env ?? '',
            health: prev?.health ?? '',
            contains: prev?.contains ?? '',
          }))
          setStage('found')
        }}
      />
    )
  }

  // Saving writes the properties and nothing else; they take effect at the
  // next deployment, and the service says so until then.
  if (stage === 'saving') {
    return (
      <PlanDialog
        request={{
          title: `Save the properties of ${service.name}`,
          url: `/api/hosts/local/instances/${container.name}/services/${service.name}/properties`,
          method: 'PUT',
          defaults: body(service),
          verb: 'Save',
        }}
        onClose={onClose}
        dirty={dirty}
        onBack={() => setStage('found')}
        onFinished={onFinished}
      />
    )
  }

  if (stage === 'deploying') {
    return (
      <PlanDialog
        request={{
          title: deployed
            ? `${changed ? 'Save and redeploy' : 'Redeploy'} ${service.name}`
            : `Deploy ${service.name} to ${container.name}`,
          url: `/api/hosts/local/instances/${container.name}/services/deploy`,
          method: 'POST',
          defaults: body(service),
          verb: deployed ? 'Redeploy' : 'Deploy',
          // Running and reachable by nothing is half done.
          // The domain is given to the service just deployed, not to the
          // container.
          next: !container.domains?.length && onAddDomain
            ? { label: 'Add a domain…', onClick: () => onAddDomain({ name: service.name }) }
            : null,
        }}
        onClose={onClose}
        dirty={dirty}
        // Back to the form, with every field as it was left.
        onBack={() => setStage('found')}
        onFinished={onFinished}
      />
    )
  }

  return (
    <Frame
      title={deployed ? `Properties of ${deployed.name}` : `Deploy a service to ${container.name}`}
      onClose={onClose}
      dirty={dirty}
    >
      {stage === 'source' && (
        <Source
          value={source}
          name={name}
          onChange={setSource}
          error={error}
          onSubmit={() => {
            const problem = repoProblem(source.repo)
            if (problem) {
              setError(problem)
              return
            }
            setError(null)
            setStage('inspecting')
          }}
        />
      )}

      {stage === 'found' && (
        <Found
          service={service}
          why={why}
          onChange={setService}
          onDeploy={() => setStage('deploying')}
          onSave={() => setStage('saving')}
          changed={changed}
          pending={Boolean(deployed?.pending)}
          // A new service was just inspected from what was typed a moment
          // ago; a typo in the branch is fixed by going back, not by closing.
          onEditSource={() => setStage('source')}
          adopted={deployed?.adopted}
          existing={Boolean(deployed)}
          peers={peers}
        />
      )}
    </Frame>
  )
}

// body is what the form sends: the same for saving and for deploying.
function body(service) {
  return {
    name: service.name,
    repo: service.repo,
    branch: service.branch,
    path: service.path,
    install: split(service.install),
    build: split(service.build),
    start: (service.start ?? '').trim(),
    runtime: service.runtime,
    port: Number(service.port) || 0,
    packages: (service.packages ?? '').split(/\s+/).filter(Boolean),
    env: toObject(service.env),
    health: { path: (service.health ?? '').trim(), contains: (service.contains ?? '').trim(), status: 0 },
  }
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
    env: '',
    health: deployed.health?.path ?? '',
    contains: deployed.health?.contains ?? '',
  }
}

// repoProblem says what is wrong with a repository URL before anything is
// cloned: only public https is supported, so anything else is said here
// rather than by git a round trip later.
function repoProblem(repo) {
  const value = (repo ?? '').trim()
  if (!value) return 'A repository URL is required.'
  if (!/^https:\/\//.test(value)) return 'Use the https address of the repository — it starts with https://. SSH addresses need a deploy key, which is not built yet.'
  return null
}

// The repository name is what anybody would have typed anyway.
function guessName(repo) {
  const last = (repo ?? '').replace(/\/+$/, '').split('/').pop() ?? ''
  return last.replace(/\.git$/, '').toLowerCase()
}

export function split(joined) {
  return joined.split('&&').map((c) => c.trim()).filter(Boolean)
}

// dirty, when true, makes closing ask first: Escape or ✕ on a form somebody
// has been filling in should not throw it away without a word.
export function Frame({ title, onClose, children, dirty }) {
  const [asking, setAsking] = useState(false)
  const leave = () => (asking ? setAsking(false) : dirty ? setAsking(true) : onClose())
  const dialog = useDialog(leave)
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-10">
      <div {...dialogProps(dialog)} className="w-full max-w-2xl rounded-lg border border-edge bg-panel shadow-2xl outline-none">
        <header className="flex items-center justify-between border-b border-edge px-5 py-3">
          <h2 id={dialog.titleId} className="text-sm font-medium">{title}</h2>
          <button onClick={leave} className="text-muted hover:text-ink" aria-label="Close">
            ✕
          </button>
        </header>
        {asking && (
          <div className="flex justify-end border-b border-edge bg-raised px-5 py-2.5">
            <Discard onKeep={() => setAsking(false)} onDiscard={onClose} />
          </div>
        )}
        {children}
      </div>
    </div>
  )
}

function Source({ value, name, onChange, error, onSubmit }) {
  // Checked when the field is left, not on every keystroke: half a URL is
  // not a mistake yet.
  const [left, setLeft] = useState(false)
  const problem = left && value.repo.trim() ? repoProblem(value.repo) : null

  return (
    <>
      <form onSubmit={(e) => { e.preventDefault(); onSubmit() }} className="space-y-3 px-5 py-4">
        <Field
          label="Repository"
          hint="A public https URL. Private repositories need a deploy key, which is not built yet."
          invalid={problem}
          onBlur={() => setLeft(true)}
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

        {error && error !== problem && (
          <p role="alert" className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
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
function Found({ service, why, onChange, onDeploy, onSave, changed, pending, onEditSource, adopted, existing, peers }) {
  const set = (key) => (v) => onChange({ ...service, [key]: v })
  const runnable = Boolean(adopted) || Boolean(service.start?.trim())
  const badNames = existing ? [] : invalidKeys(service.env)

  return (
    <>
      <div className="space-y-3 px-5 py-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <p className="text-sm">
            {service.runtime ? `Detected ${service.runtime}.` : 'Nothing recognisable at the root.'}{' '}
            <span className="text-muted">{why}</span>
          </p>
          {onEditSource && (
            <button
              onClick={onEditSource}
              className="shrink-0 text-xs text-muted underline decoration-dotted underline-offset-2 transition hover:text-ink"
            >
              Change repository or branch
            </button>
          )}
        </div>
        <p className="text-xs text-faint">
          {service.repo}
          {service.branch ? ` @ ${service.branch}` : ''}
        </p>
        <p className="text-xs text-faint">
          These are suggestions, not rules. Change anything &mdash; what you leave here is what runs,
          and what gets written onto the container.
        </p>

        <Field label="Install — has to finish" value={service.install} onChange={set("install")} mono
          hint="Run in the checkout. Several commands joined with &&." />
        <Field label="Build — has to finish" value={service.build} onChange={set("build")} mono
          hint="Left empty, no build step happens." />
        {/* An adopted service runs the way its own unit says, with its own
            environment file. Croft writes neither, so offering to edit them
            here would be offering something it will not do. */}
        {adopted?.site ? (
          <p className="rounded border border-edge px-3 py-2 text-xs text-muted">
            The build in <span className="font-mono">{adopted.output}/</span> is published to{' '}
            <span className="font-mono">{adopted.site}</span>, which the web server already serves —
            nothing is restarted. A build with no index.html is refused before anything is replaced.
            {adopted.envFile && (
              <> The build reads <span className="font-mono">{adopted.envFile}</span>, which croft never writes.</>
            )}
          </p>
        ) : adopted ? (
          <p className="rounded border border-edge px-3 py-2 text-xs text-muted">
            Runs as <span className="font-mono">{adopted.unit}</span> says
            {adopted.runAs ? <> (as <span className="font-mono">{adopted.runAs}</span>)</> : ''}, with its
            environment from <span className="font-mono">{adopted.envFile || 'nowhere'}</span>. Both stay
            exactly as they are — croft restarts the unit, and never rewrites it.
          </p>
        ) : (
          <Field label="Start — keeps running" value={service.start} onChange={set("start")} mono
            hint="What systemd runs, and restarts if it exits. The server goes here, not above." />
        )}

        <div className="grid gap-3 sm:grid-cols-2">
          {!adopted?.site && (
            <Field label="Port it listens on" type="number" value={service.port} onChange={set('port')} />
          )}
          <Field label="Packages to install first" value={service.packages} onChange={set('packages')} mono
            hint="git and curl are always installed." />
        </div>

        {/* Once a service exists its environment is its file, edited from
            Environment… and never written over by a deployment. Only the
            first deployment is given one here. */}
        {existing ? (
          <p className="text-xs text-faint">
            The environment is its own file now — change it from Environment&hellip; on the service.
          </p>
        ) : (
          <EnvEditor value={service.env} onChange={set('env')} peers={peers} />
        )}

        {/* Nothing reports readiness, so without somewhere to ask, a
            deployment is finished when the unit is up — which is a weaker
            promise, and the panel says so rather than implying more. A site
            has no process to ask at all. */}
        {!adopted?.site && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Ready when this answers" value={service.health} onChange={set('health')} mono
              placeholder="/health"
              hint={service.health.trim()
                ? 'Asked from the host, for up to 30 seconds.'
                : 'Left empty, the deployment ends when the unit starts.'} />
            <Field label="…and the body contains" value={service.contains} onChange={set('contains')} mono
              placeholder="optional" />
          </div>
        )}
      </div>

      {/* Configuring and deploying are two intentions. A service that exists
          can have its properties saved for the next deployment, or saved and
          deployed now; a new one has nothing to save until it is deployed. */}
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-edge px-5 py-3">
        <p className="text-xs text-muted">
          {!runnable
            ? 'Nothing says how to start it, so there is no deployment to run.'
            : badNames.length
              ? 'A variable name in Environment is not valid yet.'
            : existing
              ? changed
                ? 'Saving changes nothing that runs.'
                : pending ? 'Saved changes are waiting to be deployed.' : 'Nothing changed yet.'
              : 'A snapshot is taken first, so this can be undone.'}
        </p>
        <div className="flex gap-2">
          {existing && (
            <button
              onClick={onSave}
              disabled={!changed}
              className="rounded border border-edge px-3 py-1.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink disabled:opacity-40"
            >
              Save&hellip;
            </button>
          )}
          <button
            onClick={onDeploy}
            disabled={!runnable || badNames.length > 0 || (existing && !changed && !pending)}
            className="rounded border border-edge-strong bg-raised px-3 py-1.5 text-xs transition hover:border-ink/30 disabled:opacity-40"
          >
            {!existing ? 'Deploy…' : changed ? 'Save and redeploy…' : 'Redeploy…'}
          </button>
        </div>
      </footer>
    </>
  )
}

// invalid, when given, is what is wrong with the value, said under the field
// in place of the hint — where the eye already is.
export function Field({ label, hint, invalid, value, onChange, onBlur, mono, type, placeholder, autoFocus }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-muted">{label}</span>
      <input
        type={type ?? 'text'}
        value={value ?? ''}
        placeholder={placeholder}
        autoFocus={autoFocus}
        onBlur={onBlur}
        aria-invalid={invalid ? true : undefined}
        onChange={(e) => onChange(e.target.value)}
        className={`w-full rounded border bg-ground px-3 py-2 text-sm outline-none transition focus:border-ink/40 ${
          invalid ? 'border-problem/60' : 'border-field-edge'
        } ${mono ? 'font-mono text-xs' : ''}`}
      />
      {invalid ? (
        <span className="mt-1 block text-xs text-problem">{invalid}</span>
      ) : (
        hint && <span className="mt-1 block text-xs text-faint">{hint}</span>
      )}
    </label>
  )
}

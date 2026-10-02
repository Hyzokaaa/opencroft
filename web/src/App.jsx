import { useEffect, useMemo, useState } from "react"
import Shell from './components/Shell.jsx'
import Verdict from './components/Verdict.jsx'
import Findings from './components/Findings.jsx'
import InstanceTable from './components/InstanceTable.jsx'
import RouteTable from './components/RouteTable.jsx'
import Card from './components/Card.jsx'
import CertificateTable from './components/CertificateTable.jsx'
import Settings from './components/Settings.jsx'
import Login from './components/Login.jsx'
import PlanDialog from './components/PlanDialog.jsx'
import DeployDialog from './components/DeployDialog.jsx'
import AdoptDialog from './components/AdoptDialog.jsx'
import EnvDialog from './components/EnvDialog.jsx'
import Activity, { useJobs } from './components/Activity.jsx'
import Container from './components/Container.jsx'
import ProjectPage, { ProjectCards, useProjects } from './components/Projects.jsx'
import { useOverview, useCommandMode, useAuth } from './lib/useOverview.js'
import { useRoute, href } from './lib/useRoute.js'
import { VERBS } from './lib/vocabulary.js'

export default function App() {
  const { auth, refreshAuth, signOut } = useAuth()

  if (!auth) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-sm text-muted">Checking your session&hellip;</p>
      </div>
    )
  }

  // There is no sign-up: users are created on the host with `croft user add`.
  if (!auth.authenticated) {
    return <Login hasUsers={auth.hasUsers} onSignedIn={refreshAuth} />
  }

  return <Dashboard onSignOut={signOut} onSessionLost={refreshAuth} />
}

// The shape a project name has, said in the form rather than by the daemon.
const PROJECT_NAME = { pattern: '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$', invalid: 'Lowercase letters, digits and dashes, starting and ending with a letter or digit.' }
const CONTAINER_NAME = { pattern: '^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$', invalid: 'Letters, digits and dashes, starting with a letter or digit.' }

function Dashboard({ onSignOut, onSessionLost }) {
  const { data, error, fetchedAt, loading, unauthorized, reload } = useOverview()

  // The session can expire while the panel sits open.
  useEffect(() => {
    if (unauthorized) onSessionLost()
  }, [unauthorized, onSessionLost])
  const [commandMode, toggleCommands] = useCommandMode()
  const [route] = useRoute()
  const [highlighted, setHighlighted] = useState(null)
  const [dialog, setDialog] = useState(null)
  const [deploying, setDeploying] = useState(null)
  const [adopting, setAdopting] = useState(null)
  const [environment, setEnvironment] = useState(null)
  // Read again whenever the overview is: a move or a declaration shows on the
  // next poll without a second clock.
  const { projects, error: projectsError } = useProjects(data)
  const { jobs } = useJobs()
  const running = jobs.filter((j) => j.status === 'running').length

  // Subjects named by a problem, so the tables carry the same severity the
  // findings panel reports. Two truths on one screen is worse than none.
  const problems = useMemo(() => {
    const set = new Set()
    for (const f of data?.findings ?? []) {
      if (f.severity !== 'info') set.add(f.subject)
    }
    return set
  }, [data])

  const problemCount = data?.findings.filter((f) => f.severity !== 'info').length ?? 0
  const declared = (projects ?? []).filter((p) => p.declared).map((p) => p.name)

  // ── Containers ──────────────────────────────────────────────────────────────

  // A container can be born inside a project, so organising does not mean a
  // second trip to move it there.
  function newContainer(project = '') {
    setDialog({
      title: project ? `New container in ${project}` : "New container",
      url: "/api/hosts/local/instances",
      method: "POST",
      defaults: { name: "", port: 80, cpuLimit: 4, memLimit: "4GB", project },
      fields: [
        { name: "name", label: "Name", autoFocus: true, placeholder: "my-app", ...CONTAINER_NAME },
        { name: "port", label: "Port inside the container", type: "number",
          hint: "What the application listens on. The address is assigned for you." },
        { name: "cpuLimit", label: "CPU limit", type: "number" },
        { name: "memLimit", label: "Memory limit" },
        ...(declared.length
          ? [{ name: 'project', label: 'Project', optional: true,
               options: [{ value: '', label: 'No project' }, ...declared] }]
          : []),
      ],
    })
  }

  function powerContainer(instance, stop) {
    setDialog({
      title: `${stop ? 'Stop' : 'Start'} ${instance.name}`,
      url: `/api/hosts/local/instances/${instance.name}/${stop ? 'stop' : 'start'}`,
      method: 'POST',
      destructive: stop,
      verb: stop ? `Stop ${instance.name}` : `Start ${instance.name}`,
    })
  }

  // Irreversible, and the disk goes with it: the name is typed, not clicked.
  function destroyContainer(name) {
    setDialog({
      title: `${VERBS.destroy} ${name}`,
      url: `/api/hosts/local/instances/${name}`,
      method: "DELETE",
      destructive: true,
      confirm: name,
      verb: `${VERBS.destroy} ${name}`,
    })
  }

  // ── Domains ─────────────────────────────────────────────────────────────────

  function addDomain(container, domain = '') {
    setDialog({
      title: `Serve a domain from ${container.name}`,
      url: "/api/hosts/local/routes",
      method: "POST",
      defaults: { domain, target: container.name, port: container.port || 80 },
      fields: [
        { name: "domain", label: "Domain", autoFocus: true, placeholder: "app.example.com",
          hint: "It has to already point at this server. DNS is not ours to change." },
        { name: "port", label: "Port inside the container", type: "number" },
      ],
    })
  }

  function editDomain(route) {
    const current = data.instances.find((i) => i.address === route.target)
    setDialog({
      title: `Where ${route.domain} points`,
      url: `/api/hosts/local/routes/${route.domain}`,
      method: 'PUT',
      defaults: { target: current?.name ?? data.instances[0]?.name ?? '', port: route.port },
      fields: [
        { name: 'target', label: 'Container', options: data.instances.map((i) => i.name) },
        { name: 'port', label: 'Port inside the container', type: 'number',
          hint: 'The certificate is untouched — only where the traffic goes changes.' },
      ],
    })
  }

  function exposePanel(domain) {
    setDialog({
      title: `Serve this panel at ${domain}`,
      url: '/api/hosts/local/expose',
      method: 'POST',
      defaults: { domain },
    })
  }

  function enableTLS(route) {
    const domain = route.domain
    setDialog({
      title: route.ssl ? `Issue ${domain}'s certificate with croft` : `Serve ${domain} over https`,
      url: `/api/hosts/local/routes/${domain}/tls`,
      method: 'POST',
    })
  }

  // A prefix of the domain served from somewhere of its own: /api/ to a backend
  // while the web keeps the rest. Stripping is what an app mounted behind a
  // prefix it knows nothing about needs — /api/users arriving as /users.
  function addPath(route) {
    const reachable = (data?.instances ?? []).filter((i) => i.address)
    const names = reachable.map((i) => i.name)
    // Most often the API sits beside the web it serves, so the container the
    // domain already goes to is the likelier answer than the first one listed.
    const serving = reachable.find((i) => i.address === route.target)?.name
    setDialog({
      title: `Send a path of ${route.domain} elsewhere`,
      url: `/api/hosts/local/routes/${route.domain}/paths`,
      method: 'PUT',
      fields: [
        { name: 'prefix', label: 'Path', placeholder: '/api/', autoFocus: true,
          pattern: '^/([A-Za-z0-9_~-][A-Za-z0-9._~-]*/)+$',
          invalid: 'A prefix that starts and ends with a slash, like /api/.',
          hint: 'A prefix that starts and ends with a slash. The rest of the domain keeps going where it goes.' },
        { name: 'target', label: 'Container', options: names },
        { name: 'port', label: 'Port inside it', type: 'number' },
        { name: 'strip', label: 'Take the prefix off before passing it on', type: 'checkbox',
          hint: 'For a backend that answers /users, not /api/users.' },
      ],
      defaults: { prefix: '/api/', target: serving ?? names[0] ?? '', port: 3000, strip: true },
    })
  }

  function removePath(route, prefix) {
    setDialog({
      title: `Serve ${route.domain}${prefix} like the rest of it`,
      url: `/api/hosts/local/routes/${route.domain}/paths?prefix=${encodeURIComponent(prefix)}`,
      method: 'DELETE',
    })
  }

  // One certificate for a domain and every name one label below it. After it,
  // Enable https on a subdomain asks the authority for nothing.
  function wildcard() {
    setDialog({
      title: 'A wildcard certificate',
      url: '/api/hosts/local/certificates/wildcard',
      method: 'POST',
      fields: [
        { name: 'domain', label: 'Domain', placeholder: 'example.com', autoFocus: true,
          hint: 'Covers example.com and *.example.com — app.example.com, not api.app.example.com. Proved over DNS, so the credentials in Settings are used.' },
      ],
      defaults: { domain: '' },
    })
  }

  // Taking over a vhost somebody wrote by hand: croft writes its own for the
  // same domain and moves theirs aside, so the domain can have paths.
  function takeOver(domain) {
    setDialog({
      title: `${VERBS.takeOver} ${domain}`,
      url: `/api/hosts/local/routes/${domain}/takeover`,
      method: 'POST',
      verb: VERBS.takeOver,
    })
  }

  function removeDomain(domain) {
    setDialog({
      title: `Stop serving ${domain}`,
      url: `/api/hosts/local/routes/${domain}`,
      method: "DELETE",
      destructive: true,
      verb: `Stop serving ${domain}`,
    })
  }

  // ── Services ────────────────────────────────────────────────────────────────

  // Restoring reaches every service in the container and everything written
  // since, so the container's name is typed — and the summary the daemon
  // returns names what else it takes back.
  function rollback(container, snapshot) {
    setDialog({
      title: `Restore ${snapshot}`,
      url: `/api/hosts/local/instances/${container.name}/services/rollback`,
      method: 'POST',
      defaults: { snapshot },
      destructive: true,
      confirm: container.name,
      verb: 'Restore',
    })
  }

  // Everything below the snapshot it takes first is irreversible, the
  // environment file included. The daemon's summary says what else stops being
  // true — a domain pointing at its port, above all.
  function destroyService(container, service) {
    setDialog({
      title: service.adopted
        ? `${VERBS.release} ${service.name}`
        : `Remove ${service.name} from ${container.name}`,
      url: `/api/hosts/local/instances/${container.name}/services/${service.name}`,
      method: 'DELETE',
      // Releasing an adopted service removes nothing but croft's notes.
      destructive: !service.adopted,
      verb: service.adopted ? VERBS.release : `Remove ${service.name}`,
    })
  }

  // Restarting, stopping or starting touches nothing the service runs — no
  // fetch, no install, no build. Only stop leaves it down until told
  // otherwise, which is the one of the three worth pausing on.
  const POWER_TITLE = { restart: 'Restart', stop: 'Stop', start: 'Start' }
  function powerService(container, service, action) {
    setDialog({
      title: `${POWER_TITLE[action]} ${service.name}`,
      url: `/api/hosts/local/instances/${container.name}/services/${service.name}/${action}`,
      method: 'POST',
      destructive: action === 'stop',
      verb: POWER_TITLE[action],
    })
  }

  // Deploying again as it is configured: no form, straight to the plan. What is
  // deployed is what the container records, not anything this page sends.
  function redeploy(container, service) {
    setDialog({
      title: `Redeploy ${service.name}`,
      url: `/api/hosts/local/instances/${container.name}/services/${service.name}/redeploy`,
      method: 'POST',
      verb: 'Redeploy',
    })
  }

  // Same three verbs, on a unit croft found rather than deployed — a
  // different path because its name carries no croft- prefix to trust.
  function powerUnit(container, unit, action) {
    setDialog({
      title: `${POWER_TITLE[action]} ${unit.name}`,
      url: `/api/hosts/local/instances/${container.name}/units/${unit.name}/${action}`,
      method: 'POST',
      destructive: action === 'stop',
      verb: POWER_TITLE[action],
    })
  }

  // ── Projects ────────────────────────────────────────────────────────────────

  // A project is declared on the host, so it can exist before its first
  // container — organising ahead is the point.
  function newProject() {
    setDialog({
      title: 'New project',
      url: '/api/hosts/local/projects',
      method: 'POST',
      fields: [
        { name: 'name', label: 'Name', placeholder: 'my-project', autoFocus: true, ...PROJECT_NAME,
          hint: 'Lowercase letters, digits and dashes. It is written on each container in it.' },
        { name: 'description', label: 'Description', placeholder: 'What it is, in a line', optional: true },
      ],
      defaults: { name: '', description: '' },
    })
  }

  // Declares a name found on containers, or changes a declared one's
  // description: the same file either way.
  function declareProject(project) {
    setDialog({
      title: project.declared ? `Describe ${project.name}` : `Declare ${project.name}`,
      url: '/api/hosts/local/projects',
      method: 'POST',
      fields: [{ name: 'description', label: 'Description', autoFocus: true, optional: true }],
      defaults: { name: project.name, description: project.description ?? '' },
    })
  }

  function removeProject(project) {
    setDialog({
      title: `Remove the project ${project.name}`,
      url: `/api/hosts/local/projects/${project.name}`,
      method: 'DELETE',
      verb: 'Remove project',
    })
  }

  // Only the label moves: nothing restarts, nothing is copied.
  function moveToProject(container) {
    setDialog({
      title: `Move ${container.name} to a project`,
      url: `/api/hosts/local/instances/${container.name}/project`,
      method: 'PUT',
      fields: [
        { name: 'project', label: 'Project', optional: true,
          options: [{ value: '', label: 'No project' }, ...declared],
          hint: declared.length ? null : 'Create a project first, on the home page.' },
      ],
      defaults: { project: container.project ?? '' },
    })
  }

  // Bringing an existing container in from the project's side: which one is
  // chosen here, so the request names it once it is chosen.
  function addToProject(project, others) {
    const candidates = others.map((i) => i.name)
    setDialog({
      title: `Add a container to ${project.name}`,
      url: (values) => `/api/hosts/local/instances/${values.container}/project`,
      method: 'PUT',
      fields: [
        { name: 'container', label: 'Container', options: candidates,
          hint: candidates.length ? 'Only its label changes: nothing restarts.' : 'Every container is already in it.' },
      ],
      defaults: { container: candidates[0] ?? '', project: project.name },
    })
  }

  // ── Remedies ────────────────────────────────────────────────────────────────

  // What each problem's fix does, through the same dialogs as anywhere else.
  function remedy({ act, target }) {
    switch (act) {
      case 'start': return powerContainer(target, false)
      case 'open': location.hash = href('containers', target.name); return
      case 'edit-route': return editDomain(target)
      case 'remove-route': return removeDomain(target.domain)
      case 'add-route': return addDomain(target, target.domain)
      case 'take-over': return takeOver(target.domain)
      case 'issue-tls': return enableTLS(target)
    }
  }

  // A subject named in a finding: a container opens its page, a domain the
  // page of the container serving it.
  function focusSubject(subject) {
    const route = data?.routes.find((r) => r.domain === subject)
    const served = route && data.instances.find((i) => i.address === route.target)
    const name = served?.name ?? data?.instances.find((i) => i.name === subject)?.name
    location.hash = name ? href('containers', name) : href('domains')
  }

  if (!data) {
    return (
      <div className="flex min-h-screen items-center justify-center px-6">
        {error ? (
          <div className="max-w-md rounded-lg border border-problem/30 bg-panel px-5 py-4">
            <p className="text-sm text-problem">Could not reach the daemon.</p>
            <p className="mt-1 text-xs text-muted">{error}</p>
            <button
              onClick={reload}
              className="mt-3 rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs hover:border-ink/30"
            >
              Try again
            </button>
          </div>
        ) : (
          <p className="text-sm text-muted">Reading the host&hellip;</p>
        )}
      </div>
    )
  }

  const routeHandlers = {
    onRemoveDomain: removeDomain,
    onEnableTLS: enableTLS,
    onEditDomain: editDomain,
    onAddPath: addPath,
    onRemovePath: removePath,
    onTakeOver: takeOver,
  }

  const tableProps = {
    problems,
    highlighted,
    onHover: setHighlighted,
    onDestroy: destroyContainer,
    onAddDomain: addDomain,
    onPower: powerContainer,
    onDeploy: (container, service) => setDeploying({ container, service }),
    ...routeHandlers,
    instances: data.instances,
    certificates: data.certificates ?? [],
  }

  const projectActions = { newContainer, newProject, declareProject, removeProject, addToProject }

  const runtimeBin = data.runtime === 'demo' ? 'incus' : data.runtime
  const containerCommands = [`${runtimeBin} list --format json`]
  const routeCommands = ['ls /etc/nginx/croft.d/', 'nginx -T | grep server_name']

  // An address the panel no longer has, or a project with no name, is home.
  const KNOWN = ["home", "projects", "containers", "domains", "activity", "settings"]
  const { name, tab, query } = route
  const section = !KNOWN.includes(route.section) || (route.section === "projects" && !name) ? "home" : route.section
  const openedContainer = section === 'containers' && name ? data.instances.find((i) => i.name === name) : null

  // Where you are, from the section down.
  const crumbs = (() => {
    switch (section) {
      case 'projects':
        return [{ label: 'Home', href: href('home') }, { label: name, mono: true }]
      case 'containers': {
        if (!name) return [{ label: 'Containers' }]
        const project = openedContainer?.project
        return project
          ? [{ label: 'Home', href: href('home') }, { label: project, href: href('projects', project), mono: true }, { label: name, mono: true }]
          : [{ label: 'Containers', href: href('containers') }, { label: name, mono: true }]
      }
      case 'domains':
        return tab === 'certificates'
          ? [{ label: 'Domains', href: href('domains') }, { label: 'Certificates' }]
          : [{ label: 'Domains' }]
      case 'activity': return [{ label: 'Activity' }]
      case 'settings': return [{ label: 'Settings' }]
      default: return [{ label: 'Home' }]
    }
  })()

  // Filters a count on the home page opened the list with.
  const listed = data.instances.filter(
    (i) => (!query.status || (query.status === 'running' ? i.status === 'running' : i.status !== 'running')) &&
      (!query.project || i.project === query.project),
  )
  const routesListed = data.routes.filter((r) => !query.tls || r.ssl)
  const filtered = query.status || query.project || query.tls

  return (
    <Shell
      data={data}
      section={section === 'projects' ? 'home' : section}
      crumbs={crumbs}
      commandMode={commandMode}
      onToggleCommands={toggleCommands}
      freshness={freshness(fetchedAt, loading)}
      onReload={reload}
      stale={Boolean(error)}
      running={running}
      onSignOut={onSignOut}
    >
      {/* A failed poll must not blank the panel: the moment the daemon is
          shaky is the moment you most need the last known state. */}
      {error && (
        <div className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
          <span className="text-caution">No contact with the daemon.</span>
          <span className="text-muted">
            {' '}Showing the last reading{fetchedAt ? ` from ${fetchedAt.toLocaleTimeString()}` : ''}.
          </span>
        </div>
      )}

      {section === 'home' && (
        <>
          <Verdict data={data} problemCount={problemCount} />

          <Findings
            findings={data.findings}
            instances={data.instances}
            routes={data.routes}
            commandMode={commandMode}
            onFocus={focusSubject}
            onRemedy={remedy}
            checkedAt={fetchedAt ? relative(fetchedAt) : null}
          />

          <ProjectCards
            data={data}
            projects={projects}
            error={projectsError}
            problems={problems}
            actions={projectActions}
          />
        </>
      )}

      {section === 'projects' && name && (
        <ProjectPage
          name={name}
          data={data}
          projects={projects}
          problems={problems}
          commandMode={commandMode}
          actions={projectActions}
          tableProps={tableProps}
        />
      )}

      {/* One container, on its own. The list answers what exists; this
          answers what it is doing and whether it is working. */}
      {section === 'containers' && name && (
        openedContainer ? (
          <Container
            container={openedContainer}
            routes={data.routes}
            instances={data.instances}
            findings={data.findings}
            commandMode={commandMode}
            onDeploy={(container, service) => setDeploying({ container, service })}
            onRollback={rollback}
            onDestroy={destroyService}
            onPowerService={powerService}
            onRedeploy={redeploy}
            onEnvironment={(container, service) => setEnvironment({ container, service })}
            onPowerUnit={powerUnit}
            onAdopt={(container, subject) => setAdopting({ container, subject })}
            onAddDomain={addDomain}
            onMoveProject={moveToProject}
            onPower={powerContainer}
            onDestroyContainer={destroyContainer}
            routeHandlers={routeHandlers}
            onRemedy={remedy}
          />
        ) : (
          <div className="rounded-lg border border-edge bg-panel px-5 py-6 text-sm">
            <p>There is no container called <span className="font-mono">{name}</span> on this host.</p>
            <a href={href('containers')} className="mt-2 inline-block text-xs text-muted underline underline-offset-4 hover:text-ink">
              Every container
            </a>
          </div>
        )
      )}

      {section === 'containers' && !name && (
        <Card
          title={filtered ? `Containers · ${query.status ?? ''} ${query.project ?? ''}`.trim() : 'Containers'}
          count={listed.length}
          commandMode={commandMode}
          commands={containerCommands}
          action={
            <div className="flex items-center gap-3">
              {filtered && (
                <a href={href('containers')} className="text-xs text-muted transition hover:text-ink">
                  Show all
                </a>
              )}
              <NewButton onClick={() => newContainer()} />
            </div>
          }
        >
          <InstanceTable {...tableProps} instances={listed} empty={filtered ? 'Nothing matches.' : undefined} />
        </Card>
      )}

      {section === 'domains' && (
        <>
          <div role="tablist" aria-label="Domains and certificates" className="flex gap-1.5">
            <Tab on={tab !== 'certificates'} to={href('domains')}>Domains ({data.routes.length})</Tab>
            <Tab on={tab === 'certificates'} to={href('domains', 'certificates')}>
              Certificates ({data.certificates?.length ?? 0})
            </Tab>
          </div>

          {tab !== 'certificates' ? (
            <Card
              title={query.tls ? 'Domains · with TLS' : 'Domains'}
              count={routesListed.length}
              commandMode={commandMode}
              commands={routeCommands}
              action={
                query.tls && (
                  <a href={href('domains')} className="text-xs text-muted transition hover:text-ink">
                    Show all
                  </a>
                )
              }
            >
              <RouteTable {...tableProps} routes={routesListed} />
            </Card>
          ) : (
            <Card
              title="Certificates"
              count={data.certificates?.length ?? 0}
              action={
                <button
                  onClick={wildcard}
                  className="rounded border border-edge-strong bg-raised px-2 py-1 text-xs transition hover:border-ink/30"
                >
                  Wildcard certificate
                </button>
              }
              commandMode={commandMode}
              commands={["ls /etc/letsencrypt/live/", "openssl x509 -enddate -noout -in <file>"]}
            >
              <CertificateTable certificates={data.certificates ?? []} />
            </Card>
          )}
        </>
      )}

      {section === 'settings' && <Settings onExpose={exposePanel} />}

      {section === 'activity' && <Activity commandMode={commandMode} />}

      {/* Deploying has a step in the middle — look at the repository, then
          decide — so it runs its own flow and hands off to the same plan
          dialog for each half. */}
      {deploying && (
        <DeployDialog
          container={deploying.container}
          service={deploying.service}
          peers={data.instances}
          onClose={() => { setDeploying(null); reload() }}
          onFinished={reload}
        />
      )}

      {environment && (
        <EnvDialog
          container={environment.container}
          service={environment.service}
          peers={data.instances}
          onClose={() => { setEnvironment(null); reload() }}
          onFinished={reload}
        />
      )}

      {adopting && (
        <AdoptDialog
          container={adopting.container}
          subject={adopting.subject}
          onClose={() => { setAdopting(null); reload() }}
          onFinished={reload}
        />
      )}

      {dialog && (
        <PlanDialog
          request={dialog}
          onClose={() => { setDialog(null); reload() }}
          onFinished={reload}
        />
      )}
    </Shell>
  )
}

function Tab({ on, to, children }) {
  return (
    <a
      href={to}
      role="tab"
      aria-selected={on}
      className={`rounded border px-2.5 py-1 text-xs transition ${
        on ? 'border-edge-strong bg-raised text-ink' : 'border-transparent text-muted hover:text-ink'
      }`}
    >
      {children}
    </a>
  )
}

function NewButton({ onClick }) {
  return (
    <button
      onClick={onClick}
      className="rounded border border-edge-strong bg-raised px-2 py-1 text-xs transition hover:border-ink/30"
    >
      New container
    </button>
  )
}

function relative(date) {
  const seconds = Math.round((Date.now() - date.getTime()) / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  return `${Math.round(seconds / 60)}m ago`
}

function freshness(date, loading) {
  if (loading && !date) return 'loading'
  if (!date) return '—'
  return relative(date)
}

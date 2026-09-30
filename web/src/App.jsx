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
import Activity from './components/Activity.jsx'
import Container from './components/Container.jsx'
import { useOverview, useCommandMode, useAuth } from './lib/useOverview.js'

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

function Dashboard({ onSignOut, onSessionLost }) {
  const { data, error, fetchedAt, loading, unauthorized, reload } = useOverview()

  // The session can expire while the panel sits open.
  useEffect(() => {
    if (unauthorized) onSessionLost()
  }, [unauthorized, onSessionLost])
  const [commandMode, toggleCommands] = useCommandMode()
  const [section, setSection] = useState('overview')
  const [highlighted, setHighlighted] = useState(null)
  const [dialog, setDialog] = useState(null)
  const [deploying, setDeploying] = useState(null)
  const [adopting, setAdopting] = useState(null)
  const [environment, setEnvironment] = useState(null)
  const [opened, setOpened] = useState(null)

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

  function newContainer() {
    setDialog({
      title: "New container",
      url: "/api/hosts/local/instances",
      method: "POST",
      defaults: { name: "", port: 80, cpuLimit: 4, memLimit: "4GB" },
      fields: [
        { name: "name", label: "Name", autoFocus: true, placeholder: "helpdesk" },
        { name: "port", label: "Port inside the container", type: "number",
          hint: "What the application listens on. The address is assigned for you." },
        { name: "cpuLimit", label: "CPU limit", type: "number" },
        { name: "memLimit", label: "Memory limit" },
      ],
    })
  }

  function addDomain(container) {
    setDialog({
      title: `Serve a domain from ${container.name}`,
      url: "/api/hosts/local/routes",
      method: "POST",
      defaults: { domain: "", target: container.name, port: container.port || 80 },
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
      defaults: { target: current?.name ?? '', port: route.port },
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

  function enableTLS(domain) {
    setDialog({
      title: `Serve ${domain} over https`,
      url: `/api/hosts/local/routes/${domain}/tls`,
      method: 'POST',
    })
  }

  // A prefix of the domain served from somewhere of its own: /api/ to a backend
  // while the web keeps the rest. Stripping is what an app mounted behind a
  // prefix it knows nothing about needs — /api/tickets arriving as /tickets.
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
          hint: 'A prefix that starts and ends with a slash. The rest of the domain keeps going where it goes.' },
        { name: 'target', label: 'Container', options: names },
        { name: 'port', label: 'Port inside it', type: 'number' },
        { name: 'strip', label: 'Take the prefix off before passing it on', type: 'checkbox',
          hint: 'For a backend that answers /tickets, not /api/tickets.' },
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

  // Taking over a vhost somebody wrote by hand: croft writes its own for the
  // same domain and moves theirs aside, so the domain can have paths.
  function takeOver(domain) {
    setDialog({
      title: `Take over ${domain}`,
      url: `/api/hosts/local/routes/${domain}/takeover`,
      method: 'POST',
    })
  }

  function removeDomain(domain) {
    setDialog({
      title: `Stop serving ${domain}`,
      url: `/api/hosts/local/routes/${domain}`,
      method: "DELETE",
      destructive: true,
    })
  }

  function powerContainer(instance, stop) {
    setDialog({
      title: `${stop ? 'Stop' : 'Start'} ${instance.name}`,
      url: `/api/hosts/local/instances/${instance.name}/${stop ? 'stop' : 'start'}`,
      method: 'POST',
      destructive: stop,
    })
  }

  function destroyContainer(name) {
    setDialog({
      title: `Destroy ${name}`,
      url: `/api/hosts/local/instances/${name}`,
      method: "DELETE",
      destructive: true,
    })
  }

  // There is no router, but the browser's back button exists anyway and people
  // press it by reflex. Without this it leaves the application entirely
  // instead of going back to the list.
  useEffect(() => {
    if (!opened) return

    history.pushState({ opened }, '', `#/containers/${opened}`)
    const back = () => setOpened(null)

    addEventListener('popstate', back)
    return () => removeEventListener('popstate', back)
  }, [opened])

  // Restoring reaches every service in the container and everything written
  // since, so it goes through the same plan dialog as any other write — and
  // the summary the daemon returns names what else it takes back.
  function rollback(container, snapshot) {
    setDialog({
      title: `Restore ${snapshot}`,
      url: `/api/hosts/local/instances/${container.name}/services/rollback`,
      method: 'POST',
      defaults: { snapshot },
      destructive: true,
    })
  }

  // Everything below the snapshot it takes first is irreversible, the
  // environment file included. The daemon's summary says what else stops being
  // true — a domain pointing at its port, above all.
  function destroyService(container, service) {
    setDialog({
      title: service.adopted
        ? `Let go of ${service.name}`
        : `Remove ${service.name} from ${container.name}`,
      url: `/api/hosts/local/instances/${container.name}/services/${service.name}`,
      method: 'DELETE',
      // Letting go of an adopted service removes nothing but croft's notes.
      destructive: !service.adopted,
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
    })
  }

  // Deploying again as it is configured: no form, straight to the plan. What is
  // deployed is what the container records, not anything this page sends.
  function redeploy(container, service) {
    setDialog({
      title: `Redeploy ${service.name}`,
      url: `/api/hosts/local/instances/${container.name}/services/${service.name}/redeploy`,
      method: 'POST',
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
    })
  }

  function focusSubject(subject) {
    const isDomain = data?.routes.some((r) => r.domain === subject)
    if (!isDomain && data?.instances.some((i) => i.name === subject)) {
      setOpened(subject)
      return
    }

    setSection(isDomain ? 'domains' : 'containers')
    setHighlighted(subject)
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

  const tableProps = {
    problems,
    highlighted,
    onHover: setHighlighted,
    onFocus: focusSubject,
    onDestroy: destroyContainer,
    onAddDomain: addDomain,
    onPower: powerContainer,
    onDeploy: (container, service) => setDeploying({ container, service }),
    onRemoveDomain: removeDomain,
    onEnableTLS: enableTLS,
    onEditDomain: editDomain,
    onAddPath: addPath,
    onRemovePath: removePath,
    onTakeOver: takeOver,
    instances: data.instances,
  }

  const openedContainer = data.instances.find((i) => i.name === opened)

  const runtimeBin = data.runtime === 'demo' ? 'incus' : data.runtime
  const containerCommands = [`${runtimeBin} list --format json`]
  const routeCommands = ['ls /etc/nginx/croft.d/', 'nginx -T | grep server_name']

  return (
    <Shell
      data={data}
      section={opened ? "containers" : section}
      crumb={opened ? { label: opened, parent: "Containers", onParent: () => setOpened(null) } : null}
      onSection={(id) => { setOpened(null); setSection(id) }}
      commandMode={commandMode}
      onToggleCommands={toggleCommands}
      freshness={freshness(fetchedAt, loading)}
      onReload={reload}
      stale={Boolean(error)}
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

      {/* One container, on its own. The list answers what exists; this
          answers what it is doing and whether it is working. */}
      {opened && openedContainer && (
        <Container
          container={openedContainer}
          routes={data.routes}
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
        />
      )}

      {!opened && section === 'overview' && (
        <>
          <Verdict data={data} problemCount={problemCount} onFilter={setSection} />

          <Findings
            findings={data.findings}
            instances={data.instances}
            routes={data.routes}
            commandMode={commandMode}
            onFocus={focusSubject}
            checkedAt={fetchedAt ? relative(fetchedAt) : null}
          />

          {/* Hovering a domain lights up the container it serves, and back.
              That relationship is what this tool reconciles; showing it costs
              an hour and says more than any paragraph of copy. */}
          <div className="grid gap-4 xl:grid-cols-[58fr_42fr]">
            <Card
              title="Containers"
              count={data.instances.length}
              commandMode={commandMode}
              commands={containerCommands}
              action={<NewButton onClick={newContainer} />}
            >
              <InstanceTable {...tableProps} compact />
            </Card>

            <Card
              title="Domains"
              count={data.routes.length}
              commandMode={commandMode}
              commands={routeCommands}
              action={<SeeAll onClick={() => setSection('domains')} />}
            >
              <RouteTable {...tableProps} routes={data.routes} compact />
            </Card>
          </div>
        </>
      )}

      {!opened && section === 'containers' && (
        <Card
          title="Containers"
          count={data.instances.length}
          commandMode={commandMode}
          commands={containerCommands}
          action={<NewButton onClick={newContainer} />}
        >
          <InstanceTable {...tableProps} />
        </Card>
      )}

      {!opened && section === 'domains' && (
        <Card
          title="Domains"
          count={data.routes.length}
          commandMode={commandMode}
          commands={routeCommands}
        >
          <RouteTable {...tableProps} routes={data.routes} />
        </Card>
      )}

      {!opened && section === "certificates" && (
        <Card
          title="Certificates"
          count={data.certificates?.length ?? 0}
          commandMode={commandMode}
          commands={["ls /etc/letsencrypt/live/", "openssl x509 -enddate -noout -in <file>"]}
        >
          <CertificateTable certificates={data.certificates ?? []} onFocus={focusSubject} />
        </Card>
      )}

      {!opened && section === 'settings' && <Settings onExpose={exposePanel} />}

      {!opened && section === 'activity' && <Activity commandMode={commandMode} />}

      {/* Deploying has a step in the middle — look at the repository, then
          decide — so it runs its own flow and hands off to the same plan
          dialog for each half. */}
      {deploying && (
        <DeployDialog
          container={deploying.container}
          service={deploying.service}
          onClose={() => { setDeploying(null); reload() }}
          onFinished={reload}
        />
      )}

      {environment && (
        <EnvDialog
          container={environment.container}
          service={environment.service}
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

function SeeAll({ onClick }) {
  return (
    <button onClick={onClick} className="text-xs text-muted transition hover:text-ink">
      See all
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

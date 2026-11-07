import { useEffect, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Activity from './Activity.jsx'
import { StatusDot } from './InstanceTable.jsx'
import { ResourceTable, rowsOf, useResources } from './Resources.jsx'
import { PRIMARY, QUIET, SECONDARY, GHOST } from '../lib/ui.js'
import Actions from './Actions.jsx'
import { KindIcon, PlusIcon } from './Icon.jsx'
import { Status } from './Listing.jsx'
import RouteTable from './RouteTable.jsx'
import { href } from '../lib/useRoute.js'
import { allNames } from '../lib/domains.js'
import { readJSON, humane, isAgentDown } from '../lib/api.js'

// A project is the containers that belong together — a web, the backend and
// its database — and the domains that reach them. Nothing here is stored by
// the panel: a declared project is a file on the host, and membership is a
// label on each container, so what this shows is what the host says.
export function useProjects(refresh) {
  const [projects, setProjects] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    let current = true
    fetch('/api/hosts/local/projects')
      .then(readJSON)
      .then((payload) => {
        if (current) {
          setProjects(payload ?? [])
          setError(null)
        }
      })
      // The agent being down is said once, by the banner above: here the last
      // reading stays, rather than a third copy of the same failure.
      .catch((e) => current && setError(isAgentDown(e) ? null : humane(e)))
    return () => {
      current = false
    }
  }, [refresh])

  return { projects, error }
}

// containersOf is a project's containers as the overview describes them.
export function containersOf(project, instances) {
  return instances.filter((i) => i.project === project.name)
}

// routesOf is every domain with any part — the whole of it, or a path —
// served by these containers.
export function routesOf(containers, routes) {
  const addresses = new Set(containers.map((c) => c.address).filter(Boolean))
  return routes.filter((r) => addresses.has(r.target) || (r.paths ?? []).some((p) => addresses.has(p.target)))
}

// domainsOf is every domain, other name of one, or prefix of one, reaching
// these containers, with the container it reaches — which is where a click on
// it goes.
function domainsOf(containers, routes) {
  const byAddress = new Map(containers.filter((c) => c.address).map((c) => [c.address, c.name]))
  return routes.flatMap((r) => [
    ...(byAddress.has(r.target) ? [{ key: r.domain, label: r.domain, ssl: r.ssl, container: byAddress.get(r.target) }] : []),
    ...(byAddress.has(r.target) ? (r.aliases ?? []) : [])
      .map((a) => ({ key: a.domain, label: a.domain, ssl: a.ssl, container: byAddress.get(r.target) })),
    ...(r.paths ?? [])
      .filter((p) => byAddress.has(p.target))
      .map((p) => ({ key: r.domain + p.prefix, label: r.domain + p.prefix, ssl: r.ssl, container: byAddress.get(p.target) })),
  ])
}

const SMALL = 'text-xs text-muted transition hover:text-ink'

// ProjectCards is the home page's body: every project as a card, and the
// containers in none of them last.
export function ProjectCards({ data, projects, error, agentDown, problems, actions }) {
  // Every container's resources in one request, each card taking its own.
  const all = useResources()
  const rows = all.containers ? rowsOf(all.containers, data.instances, data.routes) : null
  if (error) {
    return (
      <div className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
        <span className="text-caution">Could not read the projects.</span>{' '}
        <span className="text-muted">{error}</span>
      </div>
    )
  }
  if (!projects) {
    return (
      <p role="status" className="text-xs text-muted">
        {agentDown ? 'The projects are read once the agent answers.' : <>Reading the projects&hellip;</>}
      </p>
    )
  }

  const loose = data.instances.filter((i) => !i.project)

  return (
    <section aria-labelledby="projects-heading" className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 id="projects-heading" className="text-xl font-medium">Projects</h1>
          <p className="mt-1 text-xs text-muted">
            {projects.length} project{projects.length === 1 ? '' : 's'}
            {rows && <> · {rows.filter((r) => r.kind !== 'empty' && r.kind !== 'unread').length} resources in {data.instances.length} container{data.instances.length === 1 ? '' : 's'}</>}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => actions.newContainer()} className={SECONDARY}>
            New container
          </button>
          {/* With no containers yet, the first step is the one above the
              list, and it alone stands out. */}
          <button onClick={actions.newProject} className={PRIMARY}>
            <PlusIcon /> New project
          </button>
        </div>
      </div>

      {projects.length === 0 && (
        <p className="rounded-lg border border-edge bg-panel px-4 py-6 text-center text-xs text-muted">
          No projects yet. A project groups the containers that belong together — a web, its API and
          its database — with the domains that reach them.
        </p>
      )}

      <div className="grid gap-3 lg:grid-cols-2">
        {projects.map((project) => (
          <ProjectCard
            key={project.name}
            project={project}
            containers={containersOf(project, data.instances)}
            routes={data.routes}
            problems={problems}
            actions={actions}
            others={data.instances.filter((i) => i.project !== project.name)}
            rows={rows?.filter((r) => data.instances.some((i) => i.name === r.container && i.project === project.name))}
          />
        ))}
      </div>

      {/* Containers in no project are still somebody's: listed with what
          they hold, the same way, rather than as bare names. */}
      {loose.length > 0 && (
        <section aria-labelledby="others-heading" className="space-y-3 pt-3">
          <div>
            <h2 id="others-heading" className="text-sm font-medium">Other containers</h2>
            <p className="mt-0.5 text-xs text-muted">
              In no project: {loose.length} container{loose.length === 1 ? '' : 's'}. Moving one into a project takes what it holds with it.
            </p>
          </div>
          <ResourceTable
            rows={rows?.filter((r) => loose.some((i) => i.name === r.container)) ?? null}
            error={all.error}
            empty="Nothing here."
          />
        </section>
      )}
    </section>
  )
}

// ProjectCard is one project on the home page: what it runs, each with how it
// is, what is wrong with it said first and in red, and the containers it is
// made of at the foot. Its actions fold into a menu: the card is read far
// more often than it is changed.
function ProjectCard({ project, containers, routes, problems, actions, others, rows }) {
  const troubled = containers.filter((c) => problems.has(c.name) || (c.domain && problems.has(c.domain)))
  const failing = (rows ?? []).filter((r) => r.status.tone === 'problem')
  // A container in trouble whose resources all look fine is in trouble for
  // something else — its domain, its certificate — and is named on its own.
  const otherwise = troubled.filter((c) => !failing.some((r) => r.container === c.name))
  const domains = domainsOf(containers, routes)

  return (
    <section className="flex flex-col overflow-hidden rounded-lg border border-edge bg-panel">
      <header className="flex items-start justify-between gap-3 px-4 pt-3.5 pb-3">
        <div className="min-w-0">
          <h3 className="flex flex-wrap items-center gap-2">
            <a href={href('projects', project.name)} className="font-mono text-base underline-offset-4 hover:underline">
              {project.name}
            </a>
            {/* Found on containers, declared nowhere on this host: most often
                it arrived with a migrated container. */}
            {!project.declared && (
              <Chip
                label="not declared here"
                tone="border-caution/40 text-caution"
                explain="Named by a container's label. Declaring it keeps it when it is empty."
              />
            )}
          </h3>
          {project.description && <p className="mt-0.5 text-xs text-muted">{project.description}</p>}
        </div>
        <Actions
          folded
          label={`Actions for ${project.name}`}
          actions={[
            !project.declared && { label: 'Declare', onClick: () => actions.declareProject(project) },
            { label: 'Add a container…', onClick: () => actions.addToProject(project, others) },
            { label: 'New container', onClick: () => actions.newContainer(project.name) },
          ]}
        />
      </header>

      {(failing.length > 0 || otherwise.length > 0) && (
        <ul className="border-y border-problem/20 bg-problem/[0.06] text-xs text-problem">
          {failing.map((r) => (
            <li key={r.key} className="border-l-2 border-problem px-4 py-1.5">
              <a href={r.to} className="font-mono underline-offset-4 hover:underline">{r.name}</a> is {r.status.label}
              <span className="text-muted"> in {r.container}</span>
            </li>
          ))}
          {otherwise.map((c) => (
            <li key={c.name} className="border-l-2 border-problem px-4 py-1.5">
              <a href={href('containers', c.name)} className="font-mono underline-offset-4 hover:underline">{c.name}</a> needs
              attention
            </li>
          ))}
        </ul>
      )}

      <div className="flex-1 border-t border-edge">
        {containers.length === 0 ? (
          <p className="px-4 py-3 text-xs text-muted">No containers yet.</p>
        ) : rows ? (
          <ResourceLines rows={rows} />
        ) : (
          // Until what they hold is read, the containers themselves.
          <Members containers={containers} problems={problems} />
        )}
      </div>

      {(containers.length > 0 || domains.length > 0) && (
        <footer className="space-y-1.5 border-t border-edge px-4 py-2.5">
          {domains.length > 0 && (
            <p className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-xs">
              {domains.map((d) => (
                <a key={d.key} href={href('containers', d.container)} className="text-muted underline-offset-4 hover:text-ink hover:underline">
                  <span className="text-faint">{d.ssl ? 'https://' : 'http://'}</span>
                  {d.label}
                </a>
              ))}
            </p>
          )}
          <p className="flex flex-wrap gap-x-3 gap-y-1">
            {containers.map((c) => (
              <a key={c.name} href={href('containers', c.name)} className="flex items-center gap-1.5 font-mono text-xs text-muted hover:text-ink">
                <span
                  aria-hidden="true"
                  className={`size-2 rounded-full ${c.status === 'running' ? 'bg-running' : 'border border-stopped'}`}
                />
                {c.name}
                <span className="sr-only">, {c.status}</span>
              </a>
            ))}
          </p>
        </footer>
      )}
    </section>
  )
}

// ResourceLines is a project card's body: one line per thing it runs, with
// how it is in a word. What is failing comes first, so a glance at Home is
// enough to know where to go.
function ResourceLines({ rows }) {
  const order = { problem: 0, caution: 1 }
  const sorted = [...rows].sort((a, b) => (order[a.status.tone] ?? 2) - (order[b.status.tone] ?? 2))
  return (
    <ul className="divide-y divide-edge">
      {sorted.map((r) => {
        const stopped = r.status.label === 'container stopped'
        return (
          <li key={r.key}>
            <a href={r.to} className="flex items-center justify-between gap-3 px-4 py-2 transition hover:bg-white/[0.03]">
              <span className="flex min-w-0 items-center gap-2.5">
                <KindIcon kind={r.icon ?? r.kind} className={`size-4 ${stopped ? 'text-faint' : 'text-muted'}`} />
                <span className={`truncate font-mono text-sm ${stopped ? 'text-muted' : ''}`}>
                  {r.kind === 'empty' ? <span className="font-sans text-muted">Nothing deployed in {r.container}</span> : r.name}
                </span>
                {r.chips.filter((c) => c.label === 'found here').map((c) => <Chip key={c.label} {...c} />)}
              </span>
              <Status status={{ ...r.status, explain: undefined }} />
            </a>
          </li>
        )
      })}
    </ul>
  )
}

// Members is one line per container: its state, its name, how its
// neighbours reach it. The same row in every card, grouped or not.
function Members({ containers, problems, flush }) {
  return (
    <ul className={`divide-y divide-edge/50 ${flush ? '' : 'px-4 py-1'}`}>
      {containers.map((c) => {
        const problem = problems.has(c.name) || (c.domain && problems.has(c.domain))
        return (
          <li key={c.name}>
            <a
              href={href('containers', c.name)}
              className="flex items-center justify-between gap-3 py-1.5 transition hover:text-ink"
            >
              <span className="flex min-w-0 items-center gap-2">
                <StatusDot status={c.status} problem={problem} />
                <span className="truncate font-mono text-sm">{c.name}</span>
              </span>
              <span className="truncate font-mono text-xs text-muted">{c.internalName || c.address}</span>
            </a>
          </li>
        )
      })}
    </ul>
  )
}

// ProjectPage is one project in full: what runs in it, wherever it runs, the
// domains that reach it, and what was done to it lately.
export default function ProjectPage({ name, data, projects, problems, commandMode, actions, tableProps, jobsReading }) {
  // Before any early return: a hook is called on every render or on none.
  const resources = useResources(name)
  if (!projects) return <p role="status" className="text-xs text-muted">Reading the projects&hellip;</p>
  const project = projects.find((p) => p.name === name)
  if (!project) {
    return (
      <div className="rounded-lg border border-edge bg-panel px-5 py-6 text-sm">
        <p>There is no project called <span className="font-mono">{name}</span> on this host.</p>
        <a href={href('home')} className="mt-2 inline-block text-xs text-muted underline underline-offset-4 hover:text-ink">
          Back to the projects
        </a>
      </div>
    )
  }

  const containers = containersOf(project, data.instances)
  const routes = routesOf(containers, data.routes)
  const others = data.instances.filter((i) => i.project !== project.name)
  const rows = resources.containers ? rowsOf(resources.containers, data.instances, data.routes) : null
  const count = rows?.filter((r) => r.kind !== 'empty' && r.kind !== 'unread').length

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="font-mono text-xl">{project.name}</h1>
          <p className="mt-1 text-xs text-muted">
            {rows ? `${count} resource${count === 1 ? '' : 's'} in ${containers.length} container${containers.length === 1 ? '' : 's'}` : 'Reading…'}
            {project.description ? ` · ${project.description}` : !project.declared ? ' · named by a container’s label, not declared on this host' : ''}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-4">
          <button onClick={() => actions.declareProject(project)} className={QUIET}>
            {project.declared ? 'Edit description' : 'Declare'}
          </button>
          <button onClick={() => actions.addToProject(project, others)} className={QUIET}>
            Add a container&hellip;
          </button>
          <button onClick={() => actions.newResource(project.name, containers)} className={PRIMARY}>
            <span aria-hidden="true">+</span> New resource
          </button>
        </div>
      </header>

      {/* The containers stay in sight: they are what is started, stopped,
          snapshotted and moved, even when what matters is what runs in them. */}
      {containers.length > 0 && (
        <ul aria-label="Containers" className="flex flex-wrap gap-2">
          {containers.map((c) => (
            <li key={c.name}>
              <a href={href('containers', c.name)} className="flex items-center gap-2 rounded-md border border-edge bg-panel px-3 py-1.5 text-xs transition hover:border-edge-strong">
                <StatusDot status={c.status} problem={problems.has(c.name)} />
                <span className="font-mono">{c.name}</span>
                <span className="font-mono text-faint">{c.address || c.status}</span>
              </a>
            </li>
          ))}
        </ul>
      )}

      <ResourceTable
        rows={containers.length === 0 ? [] : rows}
        error={resources.error}
        empty={
          <>
            No containers in this project yet.{' '}
            <button onClick={() => actions.newContainer(project.name)} className="underline underline-offset-4 hover:text-ink">
              Make one
            </button>{' '}
            or add one that exists.
          </>
        }
      />

      {/* What reaches the project but leads to no resource in it — a name
          left pointing at a port nothing listens on — would vanish from a
          table of resources, so the domains keep their own list. */}
      <Card title="Domains" count={allNames(routes).length}>
        <RouteTable {...tableProps} routes={routes} empty="No domain reaches this project yet. A domain is given to a service, on its page." />
      </Card>

      <Activity
        commandMode={commandMode}
        containers={containers.map((c) => c.name)}
        title="What was done here"
        limit={10}
        reading={jobsReading}
      />

      {project.declared && containers.length === 0 && (
        <Card title="Danger zone">
          <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
            <p className="text-xs text-muted">The project is empty. Removing it deletes its declaration and nothing else.</p>
            <button onClick={() => actions.removeProject(project)} className="rounded border border-problem/50 px-2.5 py-1 text-xs text-problem transition hover:bg-problem/10">
              Remove project&hellip;
            </button>
          </div>
        </Card>
      )}
    </>
  )
}


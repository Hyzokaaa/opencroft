import { useEffect, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'
import Activity from './Activity.jsx'
import InstanceTable, { StatusDot } from './InstanceTable.jsx'
import RouteTable from './RouteTable.jsx'
import { href } from '../lib/useRoute.js'
import { readJSON, humane } from '../lib/api.js'

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
      .catch((e) => current && setError(humane(e)))
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

// domainsOf is every domain, or prefix of one, reaching these containers,
// with the container it reaches — which is where a click on it goes.
function domainsOf(containers, routes) {
  const byAddress = new Map(containers.filter((c) => c.address).map((c) => [c.address, c.name]))
  return routes.flatMap((r) => [
    ...(byAddress.has(r.target) ? [{ key: r.domain, label: r.domain, ssl: r.ssl, container: byAddress.get(r.target) }] : []),
    ...(r.paths ?? [])
      .filter((p) => byAddress.has(p.target))
      .map((p) => ({ key: r.domain + p.prefix, label: r.domain + p.prefix, ssl: r.ssl, container: byAddress.get(p.target) })),
  ])
}

const SMALL = 'text-xs text-muted transition hover:text-ink'

// ProjectCards is the home page's body: every project as a card, and the
// containers in none of them last.
export function ProjectCards({ data, projects, error, problems, actions }) {
  if (error) {
    return (
      <div className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
        <span className="text-caution">Could not read the projects.</span>{' '}
        <span className="text-muted">{error}</span>
      </div>
    )
  }
  if (!projects) return <p role="status" className="text-xs text-muted">Reading the projects&hellip;</p>

  const loose = data.instances.filter((i) => !i.project)

  return (
    <section aria-labelledby="projects-heading" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="projects-heading" className="text-sm font-medium">Projects</h2>
        <div className="flex gap-2">
          <button
            onClick={() => actions.newContainer()}
            className="rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
          >
            New container
          </button>
          {/* With no containers yet, the first step is the one above the
              list, and it alone stands out. */}
          <button
            onClick={actions.newProject}
            className={
              data.instances.length
                ? 'rounded border border-edge-strong bg-raised px-2 py-1 text-xs transition hover:border-ink/30'
                : 'rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink'
            }
          >
            New project
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
          />
        ))}

        {loose.length > 0 && (
          <Card title="No project" count={loose.length}>
            <Members containers={loose} problems={problems} />
          </Card>
        )}
      </div>
    </section>
  )
}

function ProjectCard({ project, containers, routes, problems, actions, others }) {
  const domains = domainsOf(containers, routes)
  const troubled = containers.filter((c) => problems.has(c.name) || (c.domain && problems.has(c.domain)))

  return (
    <Card
      title={
        <a href={href('projects', project.name)} className="underline-offset-4 hover:underline">
          {project.name}
        </a>
      }
      count={containers.length}
      action={
        <div className="flex gap-3">
          {!project.declared && (
            <button onClick={() => actions.declareProject(project)} className={SMALL}>
              Declare
            </button>
          )}
          <button onClick={() => actions.addToProject(project, others)} className={SMALL}>
            Add a container&hellip;
          </button>
          <button onClick={() => actions.newContainer(project.name)} className={SMALL}>
            New container
          </button>
        </div>
      }
    >
      <div className="space-y-3 px-4 py-3">
        {troubled.length > 0 && (
          <p className="text-xs text-problem">
            Needs attention:{" "}
            {troubled.map((c, i) => (
              <span key={c.name}>
                {i > 0 && ", "}
                <a href={href("containers", c.name)} className="font-mono underline underline-offset-4">{c.name}</a>
              </span>
            ))}
          </p>
        )}
        {project.description && <p className="text-xs text-muted">{project.description}</p>}
        {/* Found on containers, declared nowhere on this host: most often it
            arrived with a migrated container. */}
        {!project.declared && (
          <p className="flex flex-wrap items-center gap-2 text-xs text-muted">
            <Chip label="not declared here" tone="border-caution/40 text-caution" />
            Named by a container&rsquo;s label. Declaring it keeps it when it is empty.
          </p>
        )}

        {containers.length === 0 ? (
          <p className="text-xs text-muted">No containers yet.</p>
        ) : (
          <Members containers={containers} problems={problems} flush />
        )}

        {domains.length > 0 && (
          <p className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-xs text-muted">
            {domains.map((d) => (
              <a key={d.key} href={href('containers', d.container)} className="underline-offset-4 hover:text-ink hover:underline">
                {d.ssl ? 'https://' : 'http://'}
                {d.label}
              </a>
            ))}
          </p>
        )}
      </div>
    </Card>
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

// ProjectPage is one project in full: its containers, the domains that reach
// them, the data they hold, and what was done to them lately.
export default function ProjectPage({ name, data, projects, problems, commandMode, actions, tableProps, jobsReading }) {
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

  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="font-mono text-base">{project.name}</h1>
          <p className="mt-0.5 text-xs text-muted">
            {project.description || (project.declared ? 'No description.' : 'Named by a container’s label, not declared on this host.')}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button onClick={() => actions.declareProject(project)} className="rounded border border-edge px-2.5 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink">
            {project.declared ? 'Edit description' : 'Declare'}
          </button>
          <button onClick={() => actions.addToProject(project, others)} className="rounded border border-edge px-2.5 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink">
            Add a container&hellip;
          </button>
          <button onClick={() => actions.newContainer(project.name)} className="rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30">
            New container
          </button>
        </div>
      </div>

      <Card title="Containers" count={containers.length}>
        <InstanceTable {...tableProps} instances={containers} empty="No containers in this project yet." />
      </Card>

      <Card title="Domains" count={routes.length}>
        <RouteTable {...tableProps} routes={routes} empty="No domain reaches this project yet. Add one from a container." />
      </Card>

      <Card title="Databases">
        {containers.length === 0 ? (
          <p className="px-4 py-5 text-center text-xs text-muted">No containers, so no databases.</p>
        ) : (
          <ul className="divide-y divide-edge">
            {containers.map((c) => <ProjectDatabases key={c.name} container={c} />)}
          </ul>
        )}
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

// Read once, not every few seconds for every container in the project: what
// databases exist changes when somebody adds one, and the container's own
// page is where that is watched.
function ProjectDatabases({ container }) {
  const [data, setData] = useState(null)
  useEffect(() => {
    let current = true
    // A failure is said as one, not as "none": an empty list and an unread
    // one are different answers to "where is the data".
    fetch(`/api/hosts/local/instances/${container.name}/databases`)
      .then(readJSON)
      .then((payload) => current && setData(payload ?? { databases: [] }))
      .catch((e) => current && setData({ failed: e.status === 503 ? 'not readable on this host' : 'could not be read' }))
    return () => {
      current = false
    }
  }, [container.name])
  const databases = data?.databases ?? []
  return (
    <li className="px-4 py-2.5 text-xs">
      <a href={href('containers', container.name)} className="font-mono text-muted underline-offset-4 hover:text-ink hover:underline">
        {container.name}
      </a>
      {!data ? (
        <span className="ml-2 text-faint">reading&hellip;</span>
      ) : data.failed ? (
        <span className="ml-2 text-caution">{data.failed}</span>
      ) : databases.length === 0 ? (
        <span className="ml-2 text-faint">none</span>
      ) : (
        <span className="ml-2 font-mono">
          {databases.map((d) => `${d.name} (${d.engine}${d.location ? `, in ${d.location}` : ''})`).join(' · ')}
        </span>
      )}
    </li>
  )
}

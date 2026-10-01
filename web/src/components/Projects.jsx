import { useEffect, useState } from 'react'
import Card from './Card.jsx'
import Chip from './Chip.jsx'

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
      .then(async (res) => {
        const payload = await res.json()
        if (!res.ok) throw new Error(payload.error ?? `The daemon answered ${res.status}`)
        if (current) {
          setProjects(payload ?? [])
          setError(null)
        }
      })
      .catch((e) => current && setError(e.message))
    return () => {
      current = false
    }
  }, [refresh])

  return { projects, error }
}

// domainsOf is every domain, or prefix of one, that reaches these containers.
function domainsOf(containers, routes) {
  const addresses = new Set(containers.map((c) => c.address).filter(Boolean))
  return routes.flatMap((r) => [
    ...(addresses.has(r.target) ? [{ key: r.domain, label: r.domain, ssl: r.ssl }] : []),
    ...(r.paths ?? [])
      .filter((p) => addresses.has(p.target))
      .map((p) => ({ key: r.domain + p.prefix, label: r.domain + p.prefix, ssl: r.ssl })),
  ])
}

export default function Projects({ data, projects, error, onOpen, onNew, onDeclare, onRemove }) {
  if (error) {
    return (
      <div className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
        <span className="text-caution">Could not read the projects.</span>{' '}
        <span className="text-muted">{error}</span>
      </div>
    )
  }
  if (!projects) return <p className="text-xs text-muted">Reading the projects&hellip;</p>

  const byName = new Map(data.instances.map((i) => [i.name, i]))
  const loose = data.instances.filter((i) => !i.project)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-xs text-muted">
          Containers that belong together, and the domains that reach them.
        </p>
        <button
          onClick={onNew}
          className="rounded border border-edge-strong bg-raised px-2 py-1 text-xs transition hover:border-ink/30"
        >
          New project
        </button>
      </div>

      {projects.length === 0 && (
        <p className="rounded-lg border border-edge bg-panel px-4 py-6 text-center text-xs text-muted">
          No projects yet. Create one, then move containers into it from their page.
        </p>
      )}

      {projects.map((project) => {
        const containers = project.instances.map((name) => byName.get(name)).filter(Boolean)
        const domains = domainsOf(containers, data.routes)
        return (
          <Card
            key={project.name}
            title={project.name}
            count={containers.length}
            action={
              <div className="flex gap-3">
                {!project.declared && (
                  <button onClick={() => onDeclare(project)} className="text-xs text-muted transition hover:text-ink">
                    Declare
                  </button>
                )}
                {project.declared && containers.length === 0 && (
                  <button onClick={() => onRemove(project)} className="text-xs text-muted transition hover:text-problem">
                    Remove
                  </button>
                )}
              </div>
            }
          >
            <div className="space-y-3 px-4 py-3">
              {project.description && <p className="text-xs text-muted">{project.description}</p>}
              {/* Found on containers, declared nowhere on this host: most
                  often it arrived with a migrated container. */}
              {!project.declared && (
                <p className="flex items-center gap-2 text-xs text-faint">
                  <Chip label="found, not declared" tone="border-caution/40 text-caution" />
                  Named by a container&rsquo;s label; declaring it keeps it here when it is empty.
                </p>
              )}

              {containers.length === 0 ? (
                <p className="text-xs text-faint">No containers yet — move one here from its page.</p>
              ) : (
                <ul className="divide-y divide-edge/50">
                  {containers.map((c) => (
                    <li key={c.name}>
                      <button
                        onClick={() => onOpen(c.name)}
                        className="flex w-full items-center justify-between gap-3 py-1.5 text-left transition hover:text-ink"
                      >
                        <span className="font-mono text-sm">{c.name}</span>
                        <span className="flex items-center gap-2 font-mono text-xs text-muted">
                          {c.internalName || c.address}
                          <Chip
                            label={c.status}
                            tone={c.status === 'running' ? 'border-running/40 text-running' : 'border-edge-strong text-muted'}
                          />
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}

              {domains.length > 0 && (
                <p className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-xs text-muted">
                  {domains.map((d) => (
                    <span key={d.key}>
                      {d.ssl ? 'https://' : 'http://'}
                      {d.label}
                    </span>
                  ))}
                </p>
              )}
            </div>
          </Card>
        )
      })}

      {loose.length > 0 && (
        <Card title="No project" count={loose.length}>
          <ul className="divide-y divide-edge/50 px-4 py-2">
            {loose.map((c) => (
              <li key={c.name}>
                <button
                  onClick={() => onOpen(c.name)}
                  className="flex w-full items-center justify-between gap-3 py-1.5 text-left text-muted transition hover:text-ink"
                >
                  <span className="font-mono text-sm">{c.name}</span>
                  <span className="font-mono text-xs">{c.address}</span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  )
}

import { useMemo, useState } from 'react'
import Chip from './Chip.jsx'
import Actions from './Actions.jsx'
import Chevron from './Chevron.jsx'
import { PageHead, Toolbar, Status, matches } from './Listing.jsx'
import { rowsOf, useResources } from './Resources.jsx'
import { PRIMARY } from '../lib/ui.js'
import { OWNERSHIP, VERBS } from '../lib/vocabulary.js'
import { href } from '../lib/useRoute.js'

// Every container on the host. A container is what is started, stopped,
// snapshotted and moved, so it keeps a page of its own — but each row says
// what it holds, because that is how a person recognises it.
//
// A stopped container is normal. A stopped container with a domain pointing at
// it is an incident — so the row inherits the severity of whatever finding
// names it. Two truths on one screen is worse than none.
export default function ContainersPage({ data, problems, query, onNew, onPower, onDeploy, onAddDomain, onDestroy }) {
  const resources = useResources()
  const [search, setSearch] = useState('')
  // A count on the home page opens this list already narrowed; the filter
  // starts where the link said.
  const [filter, setFilter] = useState(query.status === 'running' ? 'running' : query.status ? 'stopped' : 'all')

  const holds = useMemo(() => {
    const by = new Map()
    for (const r of rowsOf(resources.containers ?? [], data.instances, data.routes)) {
      if (r.kind === 'empty' || r.kind === 'unread') continue
      by.set(r.container, [...(by.get(r.container) ?? []), r])
    }
    return by
  }, [resources.containers, data.instances, data.routes])

  const instances = data.instances.filter((i) => !query.project || i.project === query.project)
  const running = instances.filter((i) => i.status === 'running').length
  // Split by how bad, as the rows are: a red count over amber rows said two
  // things about the same container.
  const urgent = instances.filter((i) => troubleOf(i, problems)?.severity === 'error').length
  const lookAt = instances.filter((i) => troubleOf(i, problems) && troubleOf(i, problems).severity !== 'error').length
  const filters = [
    { id: 'all', label: 'All', count: instances.length },
    { id: 'running', label: 'Running', count: running },
    { id: 'stopped', label: 'Stopped', count: instances.length - running },
  ]
  const shown = instances.filter(
    (i) =>
      (filter === 'all' || (filter === 'running') === (i.status === 'running')) &&
      matches(search, [i.name, i.project, i.address, i.image, ...(holds.get(i.name) ?? []).map((r) => r.name)]),
  )

  // The finding itself is the reason, said where the row is.
  const statusOf = (i) => {
    const finding = troubleOf(i, problems)
    return finding
      ? {
          label: finding.severity === 'error' ? 'needs attention' : 'to look at',
          tone: finding.severity === 'error' ? 'problem' : 'caution',
          explain: `${finding.subject}: ${finding.message}${finding.hint ? ` ${finding.hint}` : ''}`,
        }
      : i.status === 'running'
        ? { label: 'running', tone: 'running' }
        : { label: i.status, tone: 'muted' }
  }

  const actionsOf = (i) => [
    { label: i.status === 'running' ? 'Stop' : 'Start', onClick: () => onPower(i, i.status === 'running') },
    { label: 'Deploy', onClick: () => onDeploy(i) },
    { label: 'Add domain', onClick: () => onAddDomain(i) },
    // Destroying something croft did not create is allowed, but it is the
    // user's call and the plan says so plainly.
    { label: `${VERBS.destroy}…`, onClick: () => onDestroy(i.name), danger: true },
  ]

  return (
    <>
      <PageHead
        title="Containers"
        summary={
          <>
            {plural(instances.length, 'container')} · {running} running · {instances.length - running} stopped
            {urgent > 0 && <span className="text-problem"> · {urgent} need{urgent === 1 ? 's' : ''} attention</span>}
            {lookAt > 0 && <span className="text-caution"> · {lookAt} to look at</span>}
            {query.project && (
              <>
                {' '}· in <span className="font-mono">{query.project}</span> ·{' '}
                <a href={href('containers')} className="underline underline-offset-4 hover:text-ink">show all</a>
              </>
            )}
          </>
        }
      >
        <button onClick={onNew} className={PRIMARY}>
          <span aria-hidden="true">+</span> New container
        </button>
      </PageHead>

      <Toolbar
        query={search}
        onQuery={setSearch}
        placeholder="Search by name, project, address or what it holds"
        filters={filters}
        filter={filter}
        onFilter={setFilter}
      />

      <div className="overflow-hidden rounded-lg border border-edge bg-panel">
        {shown.length === 0 ? (
          <p className="px-4 py-6 text-center text-xs text-muted">
            {instances.length === 0 ? 'No containers on this host yet.' : 'No container matches.'}
          </p>
        ) : (
          <>
            <table className="hidden w-full text-left text-sm md:table">
              <thead className="border-b border-edge text-xs text-muted">
                <tr>
                  <th className="px-4 py-2.5 font-normal">Container</th>
                  <th className="px-4 py-2.5 font-normal">Status</th>
                  <th className="px-4 py-2.5 font-normal">Project</th>
                  <th className="hidden px-4 py-2.5 font-normal lg:table-cell">Address</th>
                  <th className="px-4 py-2.5 font-normal">Holds</th>
                  <th className="px-4 py-2.5"><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-edge/60">
                {shown.map((i) => (
                  <tr
                    key={i.name}
                    // The name is the link; the row is a larger target for a mouse.
                    onClick={() => (location.hash = href('containers', i.name))}
                    className="cursor-pointer transition-colors hover:bg-white/[0.03]"
                  >
                    <td className="relative px-4 py-3">
                      {troubleOf(i, problems) && <span className={`absolute left-0 top-0 h-full w-[2px] ${troubleOf(i, problems).severity === 'error' ? 'bg-problem' : 'bg-caution'}`} />}
                      <div className="flex flex-wrap items-center gap-2">
                        <a
                          href={href('containers', i.name)}
                          onClick={(e) => e.stopPropagation()}
                          className="font-mono font-medium underline-offset-4 hover:underline"
                        >
                          {i.name}
                        </a>
                        {!i.managed && (
                          <Chip label={OWNERSHIP.unmanaged.label} tone={OWNERSHIP.unmanaged.tone} explain={OWNERSHIP.unmanaged.explain} />
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-faint">
                        {[i.image, i.cpuLimit && `${i.cpuLimit} CPU`, i.memLimit].filter(Boolean).join(' · ')}
                      </p>
                    </td>
                    <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
                      <Status status={statusOf(i)} />
                    </td>
                    <td className="px-4 py-3 text-xs">
                      {i.project ? (
                        <a
                          href={href('projects', i.project)}
                          onClick={(e) => e.stopPropagation()}
                          className="font-mono text-muted underline-offset-4 hover:text-ink hover:underline"
                        >
                          {i.project}
                        </a>
                      ) : (
                        <span className="text-faint">&mdash;</span>
                      )}
                    </td>
                    <td className="hidden px-4 py-3 font-mono text-xs lg:table-cell">
                      {i.address || <span className="text-faint">&mdash;</span>}
                      {i.port ? <span className="text-faint">:{i.port}</span> : null}
                    </td>
                    <td className="px-4 py-3">
                      <Holds rows={holds.get(i.name)} reading={!resources.containers} />
                    </td>
                    <td className="px-4 py-3 text-right" onClick={(e) => e.stopPropagation()}>
                      <Actions folded label={`Actions for ${i.name}`} actions={actionsOf(i)} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            <ul className="divide-y divide-edge/60 md:hidden">
              {shown.map((i) => (
                <li key={i.name} className="flex items-center">
                  <a href={href('containers', i.name)} className="flex min-h-14 min-w-0 flex-1 items-center gap-3 px-4 py-3">
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate font-mono text-sm font-medium">{i.name}</span>
                        <Status status={{ ...statusOf(i), explain: undefined }} />
                      </div>
                      <p className="truncate text-xs text-muted">
                        {i.project ? <span className="font-mono">{i.project}</span> : 'no project'}
                        {' · '}
                        {resources.containers ? plural(holds.get(i.name)?.length ?? 0, 'resource') : 'reading…'}
                      </p>
                    </div>
                    <span className="text-faint" aria-hidden="true"><Chevron /></span>
                  </a>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
      {resources.error && (
        <p className="text-xs text-caution">Could not read what the containers hold: {resources.error}</p>
      )}
    </>
  )
}

function Holds({ rows, reading }) {
  if (reading) return <span className="text-xs text-faint">reading&hellip;</span>
  if (!rows?.length) return <span className="text-xs text-faint">nothing yet</span>
  return (
    <span className="flex flex-wrap gap-x-3 gap-y-1">
      {rows.map((r) => (
        <span key={r.key} className={`font-mono text-xs ${r.kind === 'found' ? 'text-faint' : 'text-muted'}`}>
          {r.name}
        </span>
      ))}
    </span>
  )
}

// troubleOf is the finding about a container or its domain, if any. problems
// maps each subject to its worst finding.
function troubleOf(i, problems) {
  return problems.get(i.name) ?? (i.domain ? problems.get(i.domain) : undefined)
}

const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`

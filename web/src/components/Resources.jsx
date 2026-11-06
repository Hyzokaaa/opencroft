import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Chip from './Chip.jsx'
import Chevron from './Chevron.jsx'
import { Toolbar, Status, matches } from './Listing.jsx'
import { href, serviceHref } from '../lib/useRoute.js'
import { portOf, reaching } from '../lib/domains.js'
import { servingOf, toneOf } from '../lib/serving.js'
import { readJSON, humane, isAgentDown } from '../lib/api.js'

// A resource is what a person thinks they have — the API, the web, the
// database — wherever it runs. The panel used to list containers and leave
// what ran in them a click away inside each one; this lists what runs, and
// says which container it is in.

const INTERVAL = 10000

// useResources reads what the containers hold. project undefined is every
// container; '' is the ones in no project.
export function useResources(project) {
  const [containers, setContainers] = useState(null)
  const [error, setError] = useState(null)
  const timer = useRef(null)

  const load = useCallback(async () => {
    const query = project === undefined ? '' : `?project=${encodeURIComponent(project)}`
    try {
      setContainers((await readJSON(await fetch(`/api/hosts/local/resources${query}`))) ?? [])
      setError(null)
    } catch (e) {
      // The agent being down is said once, by the banner: the last reading
      // stays here rather than a second copy of the same failure.
      setError(isAgentDown(e) ? null : humane(e))
    }
  }, [project])

  useEffect(() => {
    setContainers(null)
    load()
    timer.current = setInterval(load, INTERVAL)
    return () => clearInterval(timer.current)
  }, [load])

  return { containers, error, reload: load }
}

function unitStatus(state) {
  if (state === 'active') return { label: 'running', tone: 'running' }
  if (!state || state === 'unknown') return { label: 'unknown', tone: 'muted' }
  return { label: state, tone: 'problem' }
}

// rowsOf turns what each container holds into one row per resource. A
// container holding nothing is a row too: an empty box is still something
// that was made, and the place to deploy into.
export function rowsOf(containers, instances, routes) {
  const rows = []
  for (const c of containers ?? []) {
    const container = instances.find((i) => i.name === c.name) ?? { name: c.name, address: c.address, status: c.status }
    const up = c.status === 'running'
    const stopped = { label: 'container stopped', tone: 'muted', explain: `${c.name} is stopped, so nothing in it runs. What is listed is what it holds.` }
    const services = c.services?.services ?? []
    const databases = (c.databases?.databases ?? []).filter((d) => !d.location)
    const start = rows.length

    for (const s of services) {
      const site = s.adopted?.site
      const page = site ? servingOf(s) : null
      const status = !up
        ? stopped
        : page
          ? { label: page.label, tone: toneOf(page), explain: `Its front page ${page.sentence}` }
          : unitStatus(s.state)
      const names = reaching(routes, container.address, portOf(s, container))
      rows.push({
        key: `${c.name}/s/${s.name}`,
        kind: site ? 'site' : 'service',
        name: s.name,
        description: site
          ? `${site}${s.branch ? ` · built from ${s.branch}` : ''}`
          : [s.repo?.replace(/^https?:\/\/[^/]+\//, '').replace(/\.git$/, ''), s.branch && `@ ${s.branch}`, s.commit?.slice(0, 7)].filter(Boolean).join(' '),
        container: c.name,
        status,
        domain: names.find((n) => !n.prefix) ?? names[0],
        to: serviceHref(c.name, s.name),
        chips: [
          s.adopted && { label: 'adopted', tone: 'border-yours/40 text-yours' },
          s.pending && { label: 'changes not deployed', tone: 'border-yours/40 text-yours' },
          s.noSnapshot && { label: 'snapshots off', tone: 'border-edge text-faint' },
        ].filter(Boolean),
      })
    }

    for (const d of databases) {
      rows.push({
        key: `${c.name}/d/${d.name}`,
        kind: 'database',
        name: d.db || d.name,
        description: [d.engine, d.user && `user ${d.user}`, d.port && `:${d.port}`].filter(Boolean).join(' · '),
        container: c.name,
        status: up ? unitStatus(d.state) : stopped,
        to: href('containers', c.name),
        chips: [d.adopted && { label: 'adopted', tone: 'border-yours/40 text-yours' }].filter(Boolean),
      })
    }

    // What croft found running and did not deploy: listed so it is not
    // forgotten, and adopted from the container's page.
    for (const unit of c.services?.external ?? []) {
      rows.push({
        key: `${c.name}/u/${unit.name}`,
        kind: 'found',
        name: unit.name,
        description: 'A unit croft found running here and did not deploy.',
        container: c.name,
        status: up ? unitStatus(unit.state) : stopped,
        to: href('containers', c.name),
        chips: [{ label: 'found here', tone: 'border-edge-strong text-muted' }],
      })
    }
    for (const site of c.services?.sites ?? []) {
      rows.push({
        key: `${c.name}/w/${site.root}`,
        kind: 'found',
        name: site.domains?.[0] ?? site.root,
        description: `A site the container's web server serves from ${site.root}, not taken on yet.`,
        container: c.name,
        status: up ? { label: 'not checked', tone: 'muted', explain: 'Croft asks a site for its page once it is adopted.' } : stopped,
        to: href('containers', c.name),
        chips: [{ label: 'found here', tone: 'border-edge-strong text-muted' }],
      })
    }

    if (c.servicesError || c.databasesError) {
      rows.push({
        key: `${c.name}/error`,
        kind: 'unread',
        name: c.name,
        description: c.servicesError || c.databasesError,
        container: c.name,
        status: { label: 'not read', tone: 'caution' },
        to: href('containers', c.name),
        chips: [],
      })
    } else if (rows.length === start) {
      rows.push({
        key: `${c.name}/empty`,
        kind: 'empty',
        name: c.name,
        description: 'Nothing deployed here yet.',
        container: c.name,
        status: up ? { label: 'empty', tone: 'muted' } : stopped,
        to: href('containers', c.name),
        chips: [],
      })
    }
  }
  return rows
}

const KINDS = [
  { id: 'all', label: 'All' },
  { id: 'service', label: 'Services' },
  { id: 'site', label: 'Sites' },
  { id: 'database', label: 'Databases' },
  { id: 'found', label: 'Found' },
]

const KIND_LABEL = { service: 'Service', site: 'Site', database: 'Database', found: 'Found', empty: '—', unread: '—' }

// Not icons from a set nobody here maintains: a letter in a box says the
// kind as well, and reads the same in every font the panel falls back to.
function Glyph({ kind }) {
  const letter = { service: 'S', site: 'W', database: 'D', found: '?', empty: '·', unread: '!' }[kind]
  return (
    <span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-md border border-edge-strong bg-raised font-mono text-xs text-muted">
      {letter}
    </span>
  )
}

function Domain({ name }) {
  if (!name) return <span className="text-faint">—</span>
  return (
    <span className="font-mono text-xs text-muted">
      {name.ssl ? 'https://' : 'http://'}
      {name.domain}
    </span>
  )
}

// ResourceTable lists resources with a search and a filter by kind. The
// whole row opens what it names; the container is its own link.
export function ResourceTable({ rows, reading, error, showContainer = true, empty }) {
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState('all')

  const shown = useMemo(
    () =>
      (rows ?? []).filter(
        (r) => (kind === 'all' || r.kind === kind) && matches(query, [r.name, r.description, r.container, r.domain?.domain]),
      ),
    [rows, query, kind],
  )

  const present = new Set((rows ?? []).map((r) => r.kind))
  const kinds = KINDS.filter((k) => k.id === 'all' || present.has(k.id))

  return (
    <section className="space-y-3">
      <Toolbar
        query={query}
        onQuery={setQuery}
        placeholder="Search resources"
        filters={kinds.length > 2 ? kinds : null}
        filter={kind}
        onFilter={setKind}
      />

      {error && (
        <p className="rounded-lg border border-caution/30 bg-panel px-4 py-2.5 text-xs">
          <span className="text-caution">Could not read what the containers hold.</span> <span className="text-muted">{error}</span>
        </p>
      )}

      <div className="overflow-hidden rounded-lg border border-edge bg-panel">
        {!rows ? (
          <p role="status" className="px-4 py-6 text-center text-xs text-muted">
            {reading ?? <>Reading what the containers hold&hellip;</>}
          </p>
        ) : shown.length === 0 ? (
          <p className="px-4 py-6 text-center text-xs text-muted">{rows.length === 0 ? empty : 'Nothing matches.'}</p>
        ) : (
          <>
            <table className="hidden w-full text-left text-sm md:table">
              <thead className="border-b border-edge text-xs text-muted">
                <tr>
                  <th className="px-4 py-2.5 font-normal">Resource</th>
                  <th className="px-4 py-2.5 font-normal">Type</th>
                  <th className="px-4 py-2.5 font-normal">Status</th>
                  <th className="px-4 py-2.5 font-normal">Domain</th>
                  {showContainer && <th className="px-4 py-2.5 font-normal">Container</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-edge/60">
                {shown.map((r) => (
                  <tr
                    key={r.key}
                    onClick={() => (location.hash = r.to)}
                    className={`cursor-pointer transition-colors hover:bg-white/[0.03] ${r.status.label === 'container stopped' ? 'opacity-60' : ''}`}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-3">
                        <Glyph kind={r.kind} />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <a href={r.to} onClick={(e) => e.stopPropagation()} className="font-mono text-sm font-medium underline-offset-4 hover:underline">
                              {r.name}
                            </a>
                            {r.chips.map((chip) => <Chip key={chip.label} {...chip} />)}
                          </div>
                          {r.description && <p className="mt-0.5 truncate text-xs text-muted">{r.description}</p>}
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted">{KIND_LABEL[r.kind]}</td>
                    <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}><Status status={r.status} /></td>
                    <td className="px-4 py-3"><Domain name={r.domain} /></td>
                    {showContainer && (
                      <td className="px-4 py-3">
                        <a href={href('containers', r.container)} onClick={(e) => e.stopPropagation()} className="font-mono text-xs text-muted underline-offset-4 hover:text-ink hover:underline">
                          {r.container}
                        </a>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>

            {/* On a phone a row is three lines: what and how it is, what kind
                and where, and the address it answers at. */}
            <ul className="divide-y divide-edge/60 md:hidden">
              {shown.map((r) => (
                <li key={r.key} className={r.status.label === 'container stopped' ? 'opacity-60' : ''}>
                  <a href={r.to} className="flex min-h-14 items-center gap-3 px-4 py-3">
                    <Glyph kind={r.kind} />
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate font-mono text-sm font-medium">{r.name}</span>
                        <Status status={{ ...r.status, explain: undefined }} />
                      </div>
                      <p className="truncate text-xs text-muted">
                        {KIND_LABEL[r.kind]}
                        {showContainer && <> · <span className="font-mono">{r.container}</span></>}
                      </p>
                      {r.domain && <p className="truncate"><Domain name={r.domain} /></p>}
                    </div>
                    <span className="text-faint" aria-hidden="true"><Chevron /></span>
                  </a>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
      {/* Counted as resources: an empty container or one that could not be
          read is a row, not something it runs. */}
      {rows && rows.length > 0 && (
        <p className="text-xs text-faint">
          {countOf(shown) === countOf(rows) ? plural(countOf(rows)) : `${countOf(shown)} of ${plural(countOf(rows))}`}
        </p>
      )}
    </section>
  )
}

const countOf = (rows) => rows.filter((r) => r.kind !== 'empty' && r.kind !== 'unread').length
const plural = (n) => `${n} resource${n === 1 ? '' : 's'}`

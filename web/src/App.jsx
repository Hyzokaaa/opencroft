import { useMemo, useState } from 'react'
import Shell from './components/Shell.jsx'
import Verdict from './components/Verdict.jsx'
import Findings from './components/Findings.jsx'
import InstanceTable from './components/InstanceTable.jsx'
import RouteTable from './components/RouteTable.jsx'
import Card from './components/Card.jsx'
import { useOverview, useCommandMode } from './lib/useOverview.js'

export default function App() {
  const { data, error, fetchedAt, loading, reload } = useOverview()
  const [commandMode, toggleCommands] = useCommandMode()
  const [section, setSection] = useState('overview')
  const [highlighted, setHighlighted] = useState(null)

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

  function focusSubject(subject) {
    const isDomain = data?.routes.some((r) => r.domain === subject)
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
    instances: data.instances,
  }

  const runtimeBin = data.runtime === 'demo' ? 'incus' : data.runtime
  const containerCommands = [`${runtimeBin} list --format json`]
  const routeCommands = ['ls /etc/nginx/croft.d/', 'nginx -T | grep server_name']

  return (
    <Shell
      data={data}
      section={section}
      onSection={setSection}
      commandMode={commandMode}
      onToggleCommands={toggleCommands}
      freshness={freshness(fetchedAt, loading)}
      onReload={reload}
      stale={Boolean(error)}
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

      {section === 'overview' && (
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
              action={<SeeAll onClick={() => setSection('containers')} />}
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

      {section === 'containers' && (
        <Card
          title="Containers"
          count={data.instances.length}
          commandMode={commandMode}
          commands={containerCommands}
        >
          <InstanceTable {...tableProps} />
        </Card>
      )}

      {section === 'domains' && (
        <Card
          title="Domains"
          count={data.routes.length}
          commandMode={commandMode}
          commands={routeCommands}
        >
          <RouteTable {...tableProps} routes={data.routes} />
        </Card>
      )}

      {['certificates', 'activity', 'settings'].includes(section) && <NotBuilt section={section} />}
    </Shell>
  )
}

function SeeAll({ onClick }) {
  return (
    <button onClick={onClick} className="text-xs text-muted transition hover:text-ink">
      See all
    </button>
  )
}

function NotBuilt({ section }) {
  return (
    <div className="rounded-lg border border-dashed border-edge px-5 py-12 text-center">
      <p className="text-sm text-muted">{section} is not built yet.</p>
      <p className="mt-1 text-xs text-faint">
        It is on the roadmap. This panel would rather say so than invent something.
      </p>
    </div>
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

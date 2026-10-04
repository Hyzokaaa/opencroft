import { useEffect, useState } from 'react'
import { href } from '../lib/useRoute.js'
import Command from './Command.jsx'
import CopyButton from './CopyButton.jsx'
import { primaryAddress } from './AgentDown.jsx'

// Five places. Projects are the home page rather than a section beside it,
// because "is my thing working?" is asked of a project — the web, its API and
// its database — before it is asked of any one container. Certificates live
// with the domains they serve.
const SECTIONS = [
  { id: 'home', label: 'Home' },
  { id: 'containers', label: 'Containers', count: (d) => d?.instances.length },
  {
    id: 'domains', label: 'Domains', count: (d) => d?.routes.length,
    // Certificates hang off Domains: the one thing on the panel with a
    // deadline, a click away from anywhere and able to say so before it runs.
    children: [{ id: 'certificates', label: 'Certificates', to: () => href('domains', 'certificates') }],
  },
  { id: 'activity', label: 'Activity' },
  { id: 'settings', label: 'Settings' },
]

export { SECTIONS }

// crumbs is where you are, from the section down: [{ label, href }], the
// last one being this page.
export default function Shell({ data, host, section, tab, crumbs, commandMode, onToggleCommands, freshness, onReload, stale, running, onSignOut, children }) {
  const [navOpen, setNavOpen] = useState(false)

  useEffect(() => {
    if (!navOpen) return
    const onKey = (e) => e.key === 'Escape' && setNavOpen(false)
    addEventListener('keydown', onKey)
    return () => removeEventListener('keydown', onKey)
  }, [navOpen])

  // Any navigation closes the drawer: what was chosen is what you wanted to see.
  useEffect(() => {
    const close = () => setNavOpen(false)
    addEventListener('hashchange', close)
    return () => removeEventListener('hashchange', close)
  }, [])

  return (
    <div className="flex min-h-screen">
      {navOpen && (
        <div className="fixed inset-0 z-10 bg-black/50 md:hidden" onClick={() => setNavOpen(false)} aria-hidden="true" />
      )}
      <Rail data={data} host={host} commandMode={commandMode} section={section} tab={tab} open={navOpen} />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-12 shrink-0 items-center justify-between gap-4 border-b border-edge bg-ground/90 px-4 backdrop-blur md:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <button
              onClick={() => setNavOpen(!navOpen)}
              className="-ml-1 rounded p-1.5 text-muted hover:text-ink md:hidden"
              aria-label="Sections"
              aria-expanded={navOpen}
              aria-controls="sections"
            >
              <svg viewBox="0 0 16 16" className="size-4" stroke="currentColor" strokeWidth="1.5" fill="none">
                <path d="M2 4h12M2 8h12M2 12h12" strokeLinecap="round" />
              </svg>
            </button>
            <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-2">
              <span className="truncate font-mono text-xs text-muted">local</span>
              {crumbs.map((crumb, i) => {
                const last = i === crumbs.length - 1
                return (
                  <span key={crumb.href ?? crumb.label} className="flex min-w-0 items-center gap-2">
                    <span className="text-faint" aria-hidden="true">/</span>
                    {last ? (
                      <span className={`truncate text-sm ${crumb.mono ? 'font-mono' : ''}`} aria-current="page">
                        {crumb.label}
                      </span>
                    ) : (
                      <a href={crumb.href} className={`truncate text-sm text-muted transition hover:text-ink ${crumb.mono ? 'font-mono' : ''}`}>
                        {crumb.label}
                      </a>
                    )}
                  </span>
                )
              })}
            </nav>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            {/* Work left running behind a closed dialog has to stay findable. */}
            {running > 0 && (
              <a
                href={href('activity')}
                className="flex items-center gap-1.5 rounded border border-caution/40 px-2 py-1 text-xs text-caution transition hover:bg-caution/10"
              >
                <span className="size-1.5 animate-pulse rounded-full bg-caution" />
                {running} running
              </a>
            )}

            <button
              onClick={onToggleCommands}
              aria-pressed={commandMode}
              aria-label="Show commands"
              title="Show the command behind everything on screen"
              className={`rounded border px-2 py-1 font-mono text-xs transition ${
                commandMode
                  ? 'border-yours/50 bg-yours/10 text-yours'
                  : 'border-edge text-muted hover:border-edge-strong hover:text-ink'
              }`}
            >
              $_<span className="hidden font-sans sm:inline"> Commands</span>
            </button>

            <button
              onClick={onReload}
              className="flex items-center gap-1.5 rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
              title="Refresh now"
              aria-label={`Refresh now — last read ${freshness}`}
            >
              <span className={`size-1.5 rounded-full ${stale ? "bg-caution" : "bg-running"}`} />
              <span className="hidden sm:inline">{freshness}</span>
            </button>

            <button
              onClick={onSignOut}
              className="rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
            >
              Sign out
            </button>
          </div>
        </header>

        <main className="flex-1 px-4 py-5 md:px-6">
          <div className="mx-auto max-w-[1680px] space-y-4">{children}</div>
        </main>
      </div>
    </div>
  )
}

function Rail({ data, host, commandMode, section, tab, open }) {
  // Hidden off-screen on a phone is still reachable by Tab unless it is inert.
  const [narrow, setNarrow] = useState(() => matchMedia('(max-width: 767px)').matches)
  useEffect(() => {
    const query = matchMedia('(max-width: 767px)')
    const read = () => setNarrow(query.matches)
    query.addEventListener('change', read)
    return () => query.removeEventListener('change', read)
  }, [])

  return (
    <nav
      id="sections"
      aria-label="Sections"
      inert={narrow && !open ? true : undefined}
      className={`fixed inset-y-0 left-0 z-20 w-[220px] shrink-0 border-r border-edge bg-panel transition-transform md:sticky md:top-0 md:h-screen md:translate-x-0 ${
        open ? 'translate-x-0' : '-translate-x-full'
      }`}
    >
      <div className="flex h-full flex-col">
        <div className="flex h-12 shrink-0 items-center border-b border-edge px-4">
          <a href={href('home')} className="text-[15px] font-medium tracking-tight">OpenCroft</a>
        </div>

        {/* One host for now. Shown as a fact rather than a control, so it
            does not promise a choice that is not there yet. */}
        <HostAddress host={host ?? data?.host} commandMode={commandMode} />

        <ul className="min-h-0 flex-1 space-y-px overflow-y-auto p-2">
          {SECTIONS.map((s) => {
            const count = s.count?.(data)
            const here = s.id === section
            // Only one entry is the current page: on a child's page the
            // parent is lit, not marked.
            const child = here && s.children?.find((c) => c.id === tab)
            const active = here && !child
            return (
              <li key={s.id}>
                <a
                  href={href(s.id)}
                  aria-current={active ? 'page' : undefined}
                  className={`flex w-full items-center justify-between rounded px-2.5 py-1.5 text-sm transition ${
                    active ? 'bg-raised text-ink' : here ? 'text-ink hover:bg-white/[0.03]' : 'text-muted hover:bg-white/[0.03] hover:text-ink'
                  }`}
                >
                  <span>{s.label}</span>
                  {count !== undefined && <span className="text-xs text-faint">{count}</span>}
                </a>
                {s.children && (
                  <ul className="ml-4 mt-px space-y-px border-l border-edge pl-2">
                    {s.children.map((c) => {
                      const on = here && c.id === tab
                      const expiring = (data?.certificates ?? []).filter((x) => x.daysLeft < 21)
                      const expired = expiring.some((x) => x.daysLeft < 0)
                      return (
                        <li key={c.id}>
                          <a
                            href={c.to()}
                            aria-current={on ? 'page' : undefined}
                            className={`flex w-full items-center justify-between rounded px-2 py-1 text-[13px] transition ${
                              on ? 'bg-raised text-ink' : 'text-muted hover:bg-white/[0.03] hover:text-ink'
                            }`}
                          >
                            <span>{c.label}</span>
                            {/* Not a total: only what is running out, which is
                                why this is in the sidebar at all. */}
                            {c.id === 'certificates' && expiring.length > 0 && (
                              <>
                                <span
                                  aria-hidden="true"
                                  className={`text-xs font-medium ${expired ? 'text-problem' : 'text-caution'}`}
                                >
                                  {expiring.length}
                                </span>
                                <span className="sr-only"> — {expiring.length} expiring</span>
                              </>
                            )}
                          </a>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </li>
            )
          })}
        </ul>

        <div className="shrink-0 space-y-1 border-t border-edge px-4 py-3 text-[11px] text-faint">
          {data?.demo && (
            <span className="inline-block rounded border border-yours/40 px-1.5 py-px text-yours">
              demo data
            </span>
          )}
          <p className="font-mono">runtime: {data?.runtime ?? '—'}</p>
          <p className="font-mono">croft {data?.version ?? 'dev'}</p>
        </div>
      </div>
    </nav>
  )
}

// Where the server is: what somebody needs to ssh to it, or to point a domain
// at it. Public addresses first; a private one is labelled, since it only
// works from inside the same network.
function HostAddress({ host, commandMode }) {
  const addresses = host?.addresses ?? []
  const first = primaryAddress(host)
  const rest = addresses.filter((a) => a !== first)

  return (
    <div role="group" aria-label="Server address" className="max-h-[40vh] shrink-0 space-y-1 overflow-y-auto border-b border-edge px-4 py-3">
      <span className="block truncate font-mono text-xs">local</span>
      {first ? (
        <>
          <Address entry={first} />
          {rest.length > 0 && (
            <details className="text-[11px]">
              <summary className="cursor-pointer text-faint transition hover:text-ink">{rest.length} more</summary>
              <ul className="mt-1 space-y-1">
                {rest.map((a) => (
                  <li key={a.address}>
                    <Address entry={a} />
                  </li>
                ))}
              </ul>
            </details>
          )}
          {commandMode && <Command lines={[`ssh <user>@${first.address}`]} className="mt-2" />}
        </>
      ) : (
        <span className="block truncate text-[11px] text-faint">this machine</span>
      )}
    </div>
  )
}

function Address({ entry }) {
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="min-w-0 truncate font-mono text-[11px] text-muted" title={entry.address}>
        {entry.address}
      </span>
      {!entry.public && <span className="shrink-0 text-[11px] text-faint">private</span>}
      <CopyButton text={entry.address} label={`Copy ${entry.address}`} className="ml-auto shrink-0" />
    </div>
  )
}

import { useState } from 'react'

const SECTIONS = [
  { id: 'overview', label: 'Overview' },
  { id: 'containers', label: 'Containers', count: (d) => d?.instances.length },
  { id: 'domains', label: 'Domains', count: (d) => d?.routes.length },
  { id: 'certificates', label: 'Certificates' },
  { id: 'activity', label: 'Activity' },
  { id: 'settings', label: 'Settings' },
]

export { SECTIONS }

export default function Shell({ data, section, onSection, commandMode, onToggleCommands, freshness, onReload, stale, children }) {
  const [navOpen, setNavOpen] = useState(false)
  const current = SECTIONS.find((s) => s.id === section)

  return (
    <div className="flex min-h-screen">
      <Rail data={data} section={section} onSection={(id) => { onSection(id); setNavOpen(false) }} open={navOpen} />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-12 shrink-0 items-center justify-between gap-4 border-b border-edge bg-ground/90 px-4 backdrop-blur md:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <button
              onClick={() => setNavOpen(!navOpen)}
              className="-ml-1 rounded p-1.5 text-muted hover:text-ink md:hidden"
              aria-label="Sections"
            >
              <svg viewBox="0 0 16 16" className="size-4" stroke="currentColor" strokeWidth="1.5" fill="none">
                <path d="M2 4h12M2 8h12M2 12h12" strokeLinecap="round" />
              </svg>
            </button>
            <span className="truncate font-mono text-xs text-faint">local</span>
            <span className="text-faint">/</span>
            <span className="truncate text-sm">{current?.label}</span>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            <button
              onClick={onToggleCommands}
              aria-pressed={commandMode}
              title="Show the command behind everything on screen"
              className={`rounded border px-2 py-1 font-mono text-xs transition ${
                commandMode
                  ? 'border-yours/50 bg-yours/10 text-yours'
                  : 'border-edge text-muted hover:border-edge-strong hover:text-ink'
              }`}
            >
              $_
            </button>

            <button
              onClick={onReload}
              className="flex items-center gap-1.5 rounded border border-edge px-2 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
              title="Refresh now"
            >
              <span className={`size-1.5 rounded-full ${stale ? 'bg-caution' : 'bg-running'}`} />
              <span className="hidden sm:inline">{freshness}</span>
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

function Rail({ data, section, onSection, open }) {
  return (
    <nav
      className={`fixed inset-y-0 left-0 z-20 w-[220px] shrink-0 border-r border-edge bg-panel transition-transform md:static md:translate-x-0 ${
        open ? 'translate-x-0' : '-translate-x-full'
      }`}
    >
      <div className="flex h-full flex-col">
        <div className="flex h-12 items-center border-b border-edge px-4">
          <span className="text-[15px] font-medium tracking-tight">OpenCroft</span>
        </div>

        {/* The API is already /api/hosts/{id}/… — make it look like it manages
            servers from day one, even with a single entry. */}
        <div className="border-b border-edge p-3">
          <button className="flex w-full items-center justify-between rounded border border-edge bg-raised px-2.5 py-2 text-left transition hover:border-edge-strong">
            <span className="min-w-0">
              <span className="block truncate font-mono text-xs">local</span>
              <span className="block truncate text-[11px] text-faint">this machine</span>
            </span>
            <svg viewBox="0 0 16 16" className="size-3 shrink-0 text-faint" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="m4 6.5 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
        </div>

        <ul className="flex-1 space-y-px overflow-y-auto p-2">
          {SECTIONS.map((s) => {
            const count = s.count?.(data)
            const active = s.id === section
            return (
              <li key={s.id}>
                <button
                  onClick={() => onSection(s.id)}
                  aria-current={active ? 'page' : undefined}
                  className={`flex w-full items-center justify-between rounded px-2.5 py-1.5 text-sm transition ${
                    active ? 'bg-raised text-ink' : 'text-muted hover:bg-white/[0.03] hover:text-ink'
                  }`}
                >
                  <span>{s.label}</span>
                  {count !== undefined && <span className="text-xs text-faint">{count}</span>}
                </button>
              </li>
            )
          })}
        </ul>

        <div className="space-y-1 border-t border-edge px-4 py-3 text-[11px] text-faint">
          {data?.demo && (
            <span className="inline-block rounded border border-yours/40 px-1.5 py-px text-yours">
              demo data
            </span>
          )}
          <p className="font-mono">runtime: {data?.runtime ?? '—'}</p>
          <p className="font-mono">croft 0.1.0-dev</p>
        </div>
      </div>
    </nav>
  )
}

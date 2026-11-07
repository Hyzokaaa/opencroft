import { useEffect, useRef } from 'react'
import Chip from './Chip.jsx'
import Command from './Command.jsx'

// What every list page is made of, so they read the same: a heading that
// says what is there and how it is, one way to make more, a search and a
// filter above the rows, and one vocabulary of status.

// PageHead is a page's title, a line counting what it lists and what needs
// a look, and its actions — the primary one last.
export function PageHead({ title, mono, summary, children }) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className={`text-xl ${mono ? 'font-mono' : 'font-medium'}`}>{title}</h1>
        {summary && <p className="mt-1 text-xs text-muted">{summary}</p>}
      </div>
      {children && <div className="flex flex-wrap items-center gap-4">{children}</div>}
    </header>
  )
}

// Toolbar is a search and, when there is more than one way to narrow, the
// filters. Each filter says how many rows it would leave, so an empty one is
// known before it is pressed.
export function Toolbar({ query, onQuery, placeholder, filters, filter, onFilter }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label className="relative w-full min-w-0 sm:w-auto sm:max-w-sm sm:flex-1">
        <span className="sr-only">{placeholder}</span>
        <input
          type="search"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder={placeholder}
          className="h-9 w-full rounded-md border border-field-edge bg-panel px-3 text-sm placeholder:text-faint"
        />
      </label>
      {filters && filters.length > 1 && (
        <div role="group" aria-label="Show" className="flex flex-wrap gap-1">
          {filters.map((f) => (
            <button
              key={f.id}
              onClick={() => onFilter(f.id)}
              aria-pressed={filter === f.id}
              className={`h-9 rounded-md border px-3 text-xs transition ${
                filter === f.id ? 'border-edge-strong bg-raised text-ink' : 'border-edge text-muted hover:text-ink'
              }`}
            >
              {f.label}
              {f.count != null && <span className="ml-1.5 text-faint">{f.count}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// The same four tones everywhere: green is running, or a page that came
// back, and nothing else.
export const TONES = {
  running: { pill: 'border-running/40 text-running', dot: 'bg-running' },
  problem: { pill: 'border-problem/40 text-problem', dot: 'bg-problem ring-2 ring-problem/25' },
  caution: { pill: 'border-caution/40 text-caution', dot: 'border border-caution' },
  muted: { pill: 'border-edge-strong text-muted', dot: 'border border-stopped' },
}

// Status is a dot and a word — never the colour alone — with the reason one
// tap away when there is one.
export function Status({ status }) {
  const tone = TONES[status.tone] ?? TONES.muted
  return (
    <span className="inline-flex items-center gap-1.5">
      <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${tone.dot}`} />
      <Chip label={status.label} tone={tone.pill} explain={status.explain} />
    </span>
  )
}

// matches is whether any of the values contains what was typed.
export function matches(query, values) {
  const q = query.trim().toLowerCase()
  return !q || values.some((v) => v?.toLowerCase().includes(q))
}

// ReadWith is what croft runs to read the page, shown in command mode under
// the list: what the panel says can always be checked by hand.
export function ReadWith({ commandMode, commands }) {
  if (!commandMode || !commands) return null
  return (
    <section className="rounded-lg border border-edge bg-panel px-4 py-2.5">
      <p className="mb-1.5 text-[11px] uppercase tracking-wide text-faint">Read with</p>
      <Command lines={commands} />
    </section>
  )
}

// The parts of the page, as tabs under the header: a row that scrolls
// sideways inside itself on a phone, never the page.
export function Tabs({ parts, part, to, badges = {}, label }) {
  // On a phone the row is wider than the screen: the part being shown is
  // brought into view, rather than left past the edge.
  const list = useRef(null)
  useEffect(() => {
    const ul = list.current
    const here = ul?.querySelector('[aria-current="page"]')
    if (!ul || !here || ul.scrollWidth <= ul.clientWidth) return
    ul.scrollLeft = here.parentElement.offsetLeft - ul.offsetLeft - 16
  }, [part])

  return (
    <nav aria-label={label} className="min-w-0">
      <ul ref={list} className="-mx-4 flex gap-1 overflow-x-auto border-b border-edge px-4 md:mx-0 md:px-0">
        {parts.map((p) => (
          <li key={p.id} className="shrink-0">
            <a
              href={to(p.id === 'overview' ? null : p.id)}
              aria-current={p.id === part ? 'page' : undefined}
              className={`-mb-px flex items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2.5 text-sm transition ${
                p.id === part ? 'border-action text-ink' : 'border-transparent text-muted hover:text-ink'
              }`}
            >
              <span>{p.label}</span>
              <Badge badge={badges[p.id]} />
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}

function Badge({ badge }) {
  if (!badge) return null
  if (badge.dot) {
    return (
      <>
        <span aria-hidden="true" className="size-1.5 rounded-full bg-yours" />
        <span className="sr-only"> — {badge.sr}</span>
      </>
    )
  }
  return (
    <span className="text-xs text-faint">
      <span aria-hidden={badge.sr ? true : undefined}>{badge.text}</span>
      {badge.sr && <span className="sr-only">{badge.sr}</span>}
    </span>
  )
}


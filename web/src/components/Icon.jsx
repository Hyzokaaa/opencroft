// The few icons the panel uses, drawn here rather than taken from a set: each
// is a handful of strokes, and the word beside it always says the same thing.
// They never stand alone — an icon with no label next to it says nothing to a
// screen reader and too little to everyone else.

const KINDS = {
  service: <path d="M2.5 4.5 6 8l-3.5 3.5M8 12h5.5" strokeLinecap="round" strokeLinejoin="round" />,
  site: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M2.5 8h11M8 2.5c1.8 2 1.8 9 0 11M8 2.5c-1.8 2-1.8 9 0 11" />
    </>
  ),
  database: (
    <>
      <ellipse cx="8" cy="4" rx="5" ry="1.8" />
      <path d="M3 4v8c0 1 2.2 1.8 5 1.8s5-.8 5-1.8V4M3 8c0 1 2.2 1.8 5 1.8s5-.8 5-1.8" />
    </>
  ),
  container: (
    <>
      <rect x="2.5" y="3" width="11" height="10" rx="1.5" />
      <path d="M2.5 6.5h11" />
    </>
  ),
  empty: <rect x="2.5" y="3" width="11" height="10" rx="1.5" strokeDasharray="2 2" />,
  unread: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M8 5v3.5M8 11h.01" strokeLinecap="round" />
    </>
  ),
}

// KindIcon is what a resource is: a service runs a process, a site is files a
// web server hands out, a database keeps data. Found things are drawn as what
// they are; the "found here" chip says whose they are.
export function KindIcon({ kind, className = 'size-4 text-muted' }) {
  return (
    <svg viewBox="0 0 16 16" className={`shrink-0 ${className}`} fill="none" stroke="currentColor" strokeWidth="1.3" aria-hidden="true">
      {KINDS[kind] ?? KINDS.service}
    </svg>
  )
}

export function SearchIcon({ className = 'size-3.5' }) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <circle cx="7" cy="7" r="4.5" />
      <path d="M10.5 10.5 14 14" strokeLinecap="round" />
    </svg>
  )
}

export function PlusIcon({ className = 'size-3.5' }) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <path d="M8 3v10M3 8h10" strokeLinecap="round" />
    </svg>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { useDialog, dialogProps } from '../lib/useDialog.js'
import { href, serviceHref } from '../lib/useRoute.js'
import { allNames } from '../lib/domains.js'
import { readJSON } from '../lib/api.js'

// Go to… finds a page by its name: a service, a container, a domain. It only
// moves you somewhere — nothing in it starts, stops or deploys anything, so
// a slip of the keyboard here costs nothing.

const PAGES = [
  { label: 'Home', to: href('home') },
  { label: 'Containers', to: href('containers') },
  { label: 'Domains', to: href('domains') },
  { label: 'Certificates', to: href('domains', 'certificates') },
  { label: 'Activity', to: href('activity') },
  { label: 'Settings', to: href('settings') },
]

// useGoTo opens the palette with Ctrl+K, or ⌘K on a Mac.
export function useGoTo() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const onKey = (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen(true)
      }
    }
    addEventListener('keydown', onKey)
    return () => removeEventListener('keydown', onKey)
  }, [])
  return [open, setOpen]
}

export default function GoTo({ data, onClose }) {
  const dialog = useDialog(onClose)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const [held, setHeld] = useState(null)

  // What runs in the containers is read when the palette opens, not kept
  // fresh behind it: it is a way to a page, and the page reads it again.
  useEffect(() => {
    let current = true
    fetch('/api/hosts/local/resources')
      .then(readJSON)
      .then((containers) => current && setHeld(containers ?? []))
      .catch(() => current && setHeld([]))
    return () => {
      current = false
    }
  }, [])

  const items = useMemo(() => {
    const instances = data?.instances ?? []
    const projects = [...new Set(instances.map((i) => i.project).filter(Boolean))]
    const byAddress = new Map(instances.map((i) => [i.address, i.name]))
    return [
      ...(held ?? []).flatMap((c) =>
        (c.services?.services ?? []).map((s) => ({
          group: 'Resources', label: s.name, detail: `in ${c.name}`, to: serviceHref(c.name, s.name),
        })),
      ),
      ...projects.map((p) => ({ group: 'Projects', label: p, to: href('projects', p) })),
      ...instances.map((i) => ({ group: 'Containers', label: i.name, detail: i.status, to: href('containers', i.name) })),
      ...allNames(data?.routes ?? []).map((n) => ({
        group: 'Domains', label: n.domain,
        detail: byAddress.get(n.route.target) ? `to ${byAddress.get(n.route.target)}` : undefined,
        to: byAddress.has(n.route.target) ? href('containers', byAddress.get(n.route.target)) : href('domains'),
      })),
      ...PAGES.map((p) => ({ ...p, group: 'Pages' })),
    ]
  }, [data, held])

  const q = query.trim().toLowerCase()
  const shown = (q ? items.filter((i) => i.label.toLowerCase().includes(q) || i.detail?.toLowerCase().includes(q)) : items).slice(0, 40)
  const go = (item) => {
    if (!item) return
    location.hash = item.to
    onClose()
  }

  const onKey = (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, shown.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      go(shown[active])
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 px-4 pt-[12vh]" onClick={onClose}>
      <div {...dialogProps(dialog)} onClick={(e) => e.stopPropagation()} className="w-full max-w-lg overflow-hidden rounded-lg border border-edge-strong bg-panel shadow-2xl outline-none">
        <h2 id={dialog.titleId} className="sr-only">Go to</h2>
        <input
          autoFocus
          value={query}
          onChange={(e) => { setQuery(e.target.value); setActive(0) }}
          onKeyDown={onKey}
          placeholder="Go to a service, container, domain or page…"
          aria-controls="goto-results"
          aria-activedescendant={shown[active] ? `goto-${active}` : undefined}
          className="h-12 w-full border-b border-edge bg-transparent px-4 text-sm outline-none placeholder:text-faint"
        />
        <ul id="goto-results" role="listbox" className="max-h-[50vh] overflow-y-auto py-1">
          {held === null && <li role="status" className="px-4 pt-2 text-xs text-faint">Reading what runs in the containers&hellip;</li>}
          {shown.length === 0 && held !== null && <li className="px-4 py-6 text-center text-xs text-muted">Nothing called that.</li>}
          {shown.map((item, i) => (
            <li key={item.group + item.label + i}>
              {(i === 0 || shown[i - 1].group !== item.group) && (
                <p className="px-4 pb-1 pt-2 text-[11px] uppercase tracking-wide text-faint">{item.group}</p>
              )}
              <button
                id={`goto-${i}`}
                role="option"
                aria-selected={i === active}
                onMouseEnter={() => setActive(i)}
                onClick={() => go(item)}
                className={`flex w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm ${i === active ? 'bg-raised' : ''}`}
              >
                <span className="truncate font-mono">{item.label}</span>
                {item.detail && <span className="shrink-0 text-xs text-faint">{item.detail}</span>}
              </button>
            </li>
          ))}
        </ul>
        <p className="border-t border-edge px-4 py-2 text-[11px] text-faint">↑ ↓ to choose · Enter to go · Esc to close</p>
      </div>
    </div>
  )
}

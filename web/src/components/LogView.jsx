import { useEffect, useRef, useState } from 'react'
import { useLogs } from '../lib/useContainer.js'
import { useDialog, dialogProps } from '../lib/useDialog.js'

// The one thing every log viewer gets wrong is scrolling: it follows the end
// while you are trying to read something further up, and drags you away from
// it. So following stops the moment you scroll back, and says how to resume.
export default function LogView({ container, service, kind = 'service', onClose }) {
  const [follow, setFollow] = useState(true)
  const [lines, setLines] = useState(200)
  const { text, error, loading } = useLogs(container, service, { lines, follow, kind })
  const pane = useRef(null)
  const dialog = useDialog(onClose)
  const { titleId } = dialog

  // A service croft deployed always runs as croft-<name>; a unit it only
  // found keeps whatever name it already had.
  const unit = kind === 'unit' ? service : `croft-${service}`

  useEffect(() => {
    if (follow && pane.current) pane.current.scrollTop = pane.current.scrollHeight
  }, [text, follow])

  function onScroll() {
    const el = pane.current
    if (!el || !follow) return

    // Anything but the last few pixels means a person is reading.
    const atEnd = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    if (!atEnd) setFollow(false)
  }

  const body = text.trimEnd()

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 px-4 py-8"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        {...dialogProps(dialog)}
        className="flex h-full w-full max-w-4xl flex-col rounded-lg border border-edge bg-panel shadow-2xl outline-none"
      >
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-edge px-5 py-3">
          <div className="min-w-0">
            <h2 id={titleId} className="truncate text-sm font-medium">{service}</h2>
            <p className="font-mono text-[11px] text-muted">
              journalctl -u {unit} -n {lines}
            </p>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            <select
              aria-label="Lines to show"
              value={lines}
              onChange={(e) => setLines(Number(e.target.value))}
              className="rounded border border-field-edge bg-ground px-2 py-1 text-xs outline-none"
            >
              {[100, 200, 500, 1000, 2000].map((n) => (
                <option key={n} value={n}>
                  {n} lines
                </option>
              ))}
            </select>

            <button
              onClick={() => setFollow(!follow)}
              aria-pressed={follow}
              // Green says "running" on this panel and nothing else; following
              // is a choice of the viewer, so it is pressed, not green.
              className={`rounded border px-2 py-1 text-xs transition ${
                follow
                  ? 'border-edge-strong bg-raised text-ink'
                  : 'border-edge text-muted hover:border-edge-strong hover:text-ink'
              }`}
            >
              {follow ? '✓ Following' : 'Follow'}
            </button>

            <button onClick={onClose} className="-m-2 p-2 text-muted hover:text-ink" aria-label="Close">
              ✕
            </button>
          </div>
        </header>

        <div
          ref={pane}
          onScroll={onScroll}
          tabIndex={0}
          role="log"
          aria-label="journal output"
          className="min-h-0 flex-1 overflow-auto bg-ground px-4 py-3 outline-none focus-visible:ring-1 focus-visible:ring-edge-strong"
        >
          {/* A failed read keeps what was already read: the lines from two
              seconds ago are exactly what somebody was in the middle of. The
              failure is said in the footer instead. */}
          {error && !body ? (
            <p role="alert" className="text-xs text-problem">{error}</p>
          ) : loading && !body ? (
            <p className="text-xs text-muted">Reading the journal&hellip;</p>
          ) : body ? (
            <pre className="whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-muted">
              {body}
            </pre>
          ) : (
            <p className="text-xs text-muted">
              The journal has nothing for {unit}. Either it has not started yet, or it never has.
            </p>
          )}
        </div>

        <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-edge px-5 py-2.5">
          {/* Calling this a stream would be a small lie, and a person
              wondering why an entry took two seconds deserves the real
              answer. */}
          {error && body ? (
            <p role="status" className="text-[11px] text-caution">
              Could not read the journal just now: {error}{' '}
              {follow ? 'Showing the last lines read, and still asking.' : 'Showing the last lines read. Press Follow to ask again.'}
            </p>
          ) : (
            <p className="text-[11px] text-muted">
              {follow
                ? 'Asking again every two seconds. Scroll up to stop and read.'
                : 'Paused while you read. Press Follow to go back to the end.'}
            </p>
          )}
          <button
            onClick={onClose}
            className="rounded border border-edge px-3 py-1 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
          >
            Close
          </button>
        </footer>
      </div>
    </div>
  )
}

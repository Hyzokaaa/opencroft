import { useEffect, useRef, useState } from 'react'
import { useLogs } from '../lib/useContainer.js'

// The one thing every log viewer gets wrong is scrolling: it follows the end
// while you are trying to read something further up, and drags you away from
// it. So following stops the moment you scroll back, and says how to resume.
export default function LogView({ container, service, onClose }) {
  const [follow, setFollow] = useState(true)
  const [lines, setLines] = useState(200)
  const { text, error, loading } = useLogs(container, service, { lines, follow })
  const pane = useRef(null)

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
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 px-4 py-8">
      <div className="flex h-full w-full max-w-4xl flex-col rounded-lg border border-edge bg-panel shadow-2xl">
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-edge px-5 py-3">
          <div className="min-w-0">
            <h2 className="truncate text-sm font-medium">{service}</h2>
            <p className="font-mono text-[11px] text-faint">
              journalctl -u croft-{service} -n {lines}
            </p>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            <select
              value={lines}
              onChange={(e) => setLines(Number(e.target.value))}
              className="rounded border border-edge bg-ground px-2 py-1 text-xs outline-none"
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
              className={`rounded border px-2 py-1 text-xs transition ${
                follow
                  ? 'border-running/50 bg-running/10 text-running'
                  : 'border-edge text-muted hover:border-edge-strong hover:text-ink'
              }`}
            >
              {follow ? 'Following' : 'Follow'}
            </button>

            <button onClick={onClose} className="text-muted hover:text-ink" aria-label="Close">
              ✕
            </button>
          </div>
        </header>

        <div
          ref={pane}
          onScroll={onScroll}
          className="min-h-0 flex-1 overflow-auto bg-ground px-4 py-3"
        >
          {error ? (
            <p className="text-xs text-problem">{error}</p>
          ) : loading && !body ? (
            <p className="text-xs text-muted">Reading the journal&hellip;</p>
          ) : body ? (
            <pre className="whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-muted">
              {body}
            </pre>
          ) : (
            <p className="text-xs text-muted">
              The journal has nothing for croft-{service}. Either it has not started yet, or it
              never has.
            </p>
          )}
        </div>

        <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-edge px-5 py-2.5">
          {/* Calling this a stream would be a small lie, and a person
              wondering why an entry took two seconds deserves the real
              answer. */}
          <p className="text-[11px] text-faint">
            {follow
              ? 'Asking again every two seconds. Scroll up to stop and read.'
              : 'Paused while you read. Press Follow to go back to the end.'}
          </p>
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

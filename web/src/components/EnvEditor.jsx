import { useState } from 'react'

// The text is the source of truth and the table is a view of it.
//
// Built the other way round, pasting a .env from somewhere else stops working
// — and everybody pastes. So parsing happens on the way to the table and the
// text is what gets stored.
export default function EnvEditor({ value, onChange }) {
  const [asTable, setAsTable] = useState(true)
  const [revealed, setRevealed] = useState(() => new Set())

  const pairs = parse(value)

  function replace(index, key, secret) {
    const next = pairs.map((pair, i) => (i === index ? { key, value: secret } : pair))
    onChange(render(next))
  }

  function add() {
    onChange(render([...pairs, { key: '', value: '' }]))
  }

  function remove(index) {
    onChange(render(pairs.filter((_, i) => i !== index)))
  }

  function toggle(index) {
    setRevealed((current) => {
      const next = new Set(current)
      next.has(index) ? next.delete(index) : next.add(index)
      return next
    })
  }

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs text-muted">
          Environment
          {pairs.length > 0 && <span className="text-faint"> · {pairs.length}</span>}
        </span>
        <button
          type="button"
          onClick={() => setAsTable(!asTable)}
          className="text-xs text-faint transition hover:text-muted"
        >
          {asTable ? 'edit as text' : 'edit as a list'}
        </button>
      </div>

      {asTable ? (
        <div className="space-y-1.5">
          {pairs.map((pair, i) => (
            <div key={i} className="flex gap-1.5">
              <input
                value={pair.key}
                placeholder="KEY"
                onChange={(e) => replace(i, e.target.value, pair.value)}
                className="w-2/5 rounded border border-edge bg-ground px-2 py-1.5 font-mono text-xs outline-none transition focus:border-edge-strong"
              />
              <input
                // Masked by default: it is your own server and you can read
                // this file over ssh whenever you like, but a shared screen
                // should not give it away by accident.
                type={revealed.has(i) ? 'text' : 'password'}
                value={pair.value}
                placeholder="value"
                onChange={(e) => replace(i, pair.key, e.target.value)}
                className="min-w-0 flex-1 rounded border border-edge bg-ground px-2 py-1.5 font-mono text-xs outline-none transition focus:border-edge-strong"
              />
              <button
                type="button"
                onClick={() => toggle(i)}
                aria-label={revealed.has(i) ? 'Hide' : 'Show'}
                className="rounded border border-edge px-2 text-xs text-faint transition hover:border-edge-strong hover:text-muted"
              >
                {revealed.has(i) ? '⦻' : '⦾'}
              </button>
              <button
                type="button"
                onClick={() => remove(i)}
                aria-label="Remove"
                className="rounded border border-edge px-2 text-xs text-faint transition hover:border-problem/50 hover:text-problem"
              >
                ✕
              </button>
            </div>
          ))}

          <button
            type="button"
            onClick={add}
            className="rounded border border-dashed border-edge px-2 py-1 text-xs text-faint transition hover:border-edge-strong hover:text-muted"
          >
            + variable
          </button>
        </div>
      ) : (
        <textarea
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={Math.max(6, pairs.length + 2)}
          spellCheck={false}
          placeholder={'JWT_SECRET=…\nDB_HOST=postgres'}
          className="w-full rounded border border-edge bg-ground px-3 py-2 font-mono text-xs outline-none transition focus:border-edge-strong"
        />
      )}

      <p className="mt-1 text-xs text-faint">
        Written to <code>.env</code> beside the code, readable only by its owner, and read by the
        unit at start.
      </p>
    </div>
  )
}

// Lines that are not KEY=value are kept as they are in the text and simply do
// not appear in the table. Throwing away what somebody pasted would be worse
// than showing less of it.
export function parse(text) {
  const pairs = []

  for (const line of (text ?? '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue

    const at = trimmed.indexOf('=')
    if (at < 1) continue

    pairs.push({
      key: trimmed.slice(0, at).trim(),
      value: unquote(trimmed.slice(at + 1).trim()),
    })
  }
  return pairs
}

function unquote(value) {
  const quoted = value.length > 1 && (value.startsWith('"') || value.startsWith("'"))
  return quoted && value.at(-1) === value[0] ? value.slice(1, -1) : value
}

function render(pairs) {
  return pairs.map(({ key, value }) => `${key}=${value}`).join('\n')
}

// toObject is what crosses the wire: the agent checks every key and value
// again, because it cannot assume the panel is the one calling.
export function toObject(text) {
  const env = {}
  for (const { key, value } of parse(text)) {
    if (key) env[key] = value
  }
  return env
}

export function fromObject(env) {
  return Object.entries(env ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

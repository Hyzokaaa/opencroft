import { useState } from 'react'

// The text is the source of truth and the table is a view of it.
//
// Built the other way round, pasting a .env from somewhere else stops working
// — and everybody pastes. So parsing happens on the way to the table and the
// text is what gets stored.
export default function EnvEditor({ value, onChange, note, peers = [] }) {
  const [asTable, setAsTable] = useState(true)
  // Revealed by name, not by position: removing the row above one that was
  // shown used to reveal whichever secret slid into its place.
  const [revealed, setRevealed] = useState(() => new Set())

  const pairs = parse(value)
  const suggestions = byName(pairs, peers)
  const { ignored, duplicates } = lint(value)

  // Swapping an address for a name touches that address only, wherever it
  // appears in the value — a URL keeps its user, password, port and path.
  function swapForName({ index, address, name }) {
    const pair = pairs[index]
    replace(index, pair.key, pair.value.replace(wholeAddress(address, 'g'), `$1${name}`))
  }

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

  function toggle(key) {
    setRevealed((current) => {
      const next = new Set(current)
      next.has(key) ? next.delete(key) : next.add(key)
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
        <div className="flex overflow-hidden rounded border border-edge text-xs">
          {[
            [true, 'List'],
            [false, 'Text'],
          ].map(([table, label]) => (
            <button
              key={label}
              type="button"
              onClick={() => setAsTable(table)}
              className={`px-2 py-0.5 transition ${
                asTable === table ? 'bg-raised text-ink' : 'text-faint hover:text-muted'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {asTable ? (
        <div className="space-y-1.5">
          {pairs.map((pair, i) => {
            // Each field is named after its variable: eight inputs all called
            // "edit text" leave a screen reader with nothing to go on.
            const called = pair.key || `variable ${i + 1}`
            const shown = revealed.has(pair.key || `#${i}`)
            const wrong = !validPair(pair)
            return (
              <div key={i} className="flex gap-1.5">
                <input
                  value={pair.key}
                  placeholder="KEY"
                  aria-label={`Name of ${called}`}
                  aria-invalid={wrong || undefined}
                  onChange={(e) => replace(i, e.target.value, pair.value)}
                  className={`w-2/5 rounded border bg-ground px-2 py-1.5 font-mono text-xs outline-none transition focus:border-ink/40 ${
                    wrong ? 'border-problem/60' : 'border-field-edge'
                  }`}
                />
                <input
                  // Masked by default: it is your own server and you can read
                  // this file over ssh whenever you like, but a shared screen
                  // should not give it away by accident.
                  type={shown ? 'text' : 'password'}
                  value={pair.value}
                  placeholder="value"
                  aria-label={`Value of ${called}`}
                  autoComplete="off"
                  onChange={(e) => replace(i, pair.key, e.target.value)}
                  className="min-w-0 flex-1 rounded border border-field-edge bg-ground px-2 py-1.5 font-mono text-xs outline-none transition focus:border-ink/40"
                />
                {/* A word, not a glyph: ⦾ and ⦻ meant nothing until pressed. */}
                <button
                  type="button"
                  onClick={() => toggle(pair.key || `#${i}`)}
                  aria-label={`${shown ? 'Hide' : 'Show'} ${called}`}
                  className="w-12 shrink-0 rounded border border-edge px-2 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
                >
                  {shown ? 'hide' : 'show'}
                </button>
                <button
                  type="button"
                  onClick={() => remove(i)}
                  aria-label={`Remove ${called}`}
                  className="rounded border border-edge px-2 text-xs text-faint transition hover:border-problem/50 hover:text-problem"
                >
                  ✕
                </button>
              </div>
            )
          })}

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
          placeholder={'SECRET_KEY=…\nDATABASE_URL=…'}
          className="w-full rounded border border-field-edge bg-ground px-3 py-2 font-mono text-xs outline-none transition focus:border-ink/40"
        />
      )}

      {/* Said under the editor in either view: the text is where these are
          typed, and the list is where their absence would go unnoticed. */}
      {pairs.some((p) => !validPair(p)) && (
        <p className="mt-2 text-xs text-problem">
          A name is letters, digits and underscores, not starting with a digit — and a value needs
          one.
        </p>
      )}
      {(ignored.length > 0 || duplicates.length > 0) && (
        <div className="mt-2 space-y-1 rounded border border-caution/30 bg-caution/[0.06] px-3 py-2 text-xs text-caution">
          {ignored.length > 0 && (
            <p>
              {ignored.length === 1 ? 'This line is' : 'These lines are'} not NAME=value, so{' '}
              {ignored.length === 1 ? 'it is' : 'they are'} left out of what is saved:{' '}
              <span className="font-mono">{ignored.join(', ')}</span>
            </p>
          )}
          {duplicates.length > 0 && (
            <p>
              Set more than once, so only the last one is saved:{' '}
              <span className="font-mono">{duplicates.join(', ')}</span>
            </p>
          )}
        </div>
      )}

      {suggestions.length > 0 && (
        <ul className="mt-2 space-y-1 rounded border border-edge bg-ground px-3 py-2 text-xs">
          {suggestions.map((s) => (
            <li key={`${s.index}-${s.address}`} className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-muted">
                <span className="font-mono text-ink">{s.key}</span> reaches{' '}
                <span className="font-mono">{s.peer}</span> by its address, which changes if the container is rebuilt.
              </span>
              <button
                type="button"
                onClick={() => swapForName(s)}
                className="rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
              >
                Use {s.name}
              </button>
            </li>
          ))}
        </ul>
      )}

      <p className="mt-1 text-xs text-faint">
        {note ?? (
          <>
            Written to <code>.env</code> beside the code, readable only by its owner, and read by
            the unit at start. After this first deployment, change it from Environment&hellip;
          </>
        )}
      </p>
    </div>
  )
}

// byName finds values that reach another container by its address when the
// bridge would answer to its name. Only whole addresses count: 10.0.0.5 must
// not match inside 10.0.0.50.
function byName(pairs, peers) {
  const found = []
  pairs.forEach((pair, index) => {
    for (const peer of peers) {
      if (!peer.address || !peer.internalName) continue
      if (wholeAddress(peer.address).test(pair.value)) {
        found.push({ index, key: pair.key, address: peer.address, peer: peer.name, name: peer.internalName })
      }
    }
  })
  return found
}

// Lines that are not KEY=value are kept as they are in the text and simply do
// not appear in the table. Throwing away what somebody pasted would be worse
// than showing less of it.
export function parse(text) {
  const pairs = []

  for (const line of (text ?? '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue

    // A line with no key yet is a row being filled in: dropping it would make
    // "+ variable" add nothing. toObject is what leaves it out of the file.
    const at = trimmed.indexOf('=')
    if (at < 0) continue

    pairs.push({
      key: name(trimmed.slice(0, at)),
      value: unquote(trimmed.slice(at + 1).trim()),
    })
  }
  return pairs
}

// A file written for a shell says `export NAME=value`. The word is the
// shell's, not part of the name, and systemd would not take it either.
function name(raw) {
  return raw.trim().replace(/^export\s+/, '')
}

// lint finds what a paste brings that the file cannot hold: lines that are not
// NAME=value, and names set twice — where only the last one survives.
export function lint(text) {
  const ignored = []
  const seen = new Map()
  for (const line of (text ?? '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const at = trimmed.indexOf('=')
    if (at < 0) {
      ignored.push(trimmed)
      continue
    }
    const key = name(trimmed.slice(0, at))
    if (key) seen.set(key, (seen.get(key) ?? 0) + 1)
  }
  return { ignored, duplicates: [...seen].filter(([, n]) => n > 1).map(([key]) => key) }
}

// The shape systemd and every shell accept. A row still being filled in —
// no name, no value — is not wrong yet; a value with no name is.
const KEY = /^[A-Za-z_][A-Za-z0-9_]*$/
function validPair({ key, value }) {
  return key ? KEY.test(key) : value === ''
}

// invalidKeys is what has to be fixed before a save means anything.
export function invalidKeys(text) {
  return parse(text).filter((p) => !validPair(p)).map((p) => p.key || '(no name)')
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

function wholeAddress(address, flags = '') {
  return new RegExp(`(^|[^0-9.])${address.replaceAll('.', '\\.')}(?![0-9])`, flags)
}

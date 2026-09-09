import { useState } from 'react'

// Commands in ink, comments in muted. Painting the product's main argument in
// the faintest colour on the page was a mistake.
export default function Command({ lines, className = '' }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(lines.join('\n'))
      setCopied(true)
      setTimeout(() => setCopied(false), 1400)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className={`group relative rounded border border-edge bg-ground ${className}`}>
      <pre className="overflow-x-auto px-3 py-2 font-mono text-xs leading-relaxed">
        {lines.map((line, i) => (
          <div key={i} className={line.startsWith('#') ? 'text-faint' : 'text-ink'}>
            {line || ' '}
          </div>
        ))}
      </pre>
      <button
        onClick={copy}
        className="absolute right-1.5 top-1.5 rounded border border-edge bg-panel px-1.5 py-0.5 text-[11px] text-muted opacity-0 transition hover:text-ink focus-visible:opacity-100 group-hover:opacity-100"
      >
        {copied ? 'copied' : 'copy'}
      </button>
    </div>
  )
}

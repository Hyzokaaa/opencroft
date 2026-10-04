import { useEffect, useRef, useState } from 'react'

// One copy button for the panel: a command, an address. Visible, if quiet,
// and big enough to hit (24px, WCAG 2.5.8). The outcome is announced, since a
// word changing inside a button is not news to a screen reader.
export default function CopyButton({ text, label, className = '' }) {
  const [copied, setCopied] = useState(false)
  const timer = useRef(null)

  useEffect(() => () => clearTimeout(timer.current), [])

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => setCopied(false), 1400)
    } catch {
      setCopied(false)
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={copy}
        aria-label={label}
        className={`inline-flex min-h-6 min-w-6 items-center justify-center rounded border border-edge bg-panel px-1.5 py-0.5 text-[11px] text-muted transition hover:text-ink ${className}`}
      >
        {copied ? 'copied' : 'copy'}
      </button>
      <span aria-live="polite" className="sr-only">
        {copied ? 'Copied' : ''}
      </span>
    </>
  )
}

import { useId, useState } from 'react'

// This explanation used to live in a title attribute — invisible on touch,
// invisible to the keyboard, and it happened to hold the product's pitch.
//
// pill is the shape of a status, with dot the class of the dot inside it: a
// state reads as one object — a dot and a word — not two things beside each
// other that wrap apart.
export default function Chip({ label, tone, explain, pill = false, dot }) {
  const [open, setOpen] = useState(false)
  const id = useId()

  const shape = pill
    ? 'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2 py-0.5 text-xs leading-4'
    : 'inline-block rounded border px-1.5 py-px text-[11px] leading-4'
  const body = (
    <>
      {dot && <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dot}`} />}
      {label}
    </>
  )

  // With nothing to explain it is a label, not a control: a button there is
  // one more empty stop on the way through the page with Tab.
  if (!explain) {
    return <span className={`${shape} ${tone ?? ''}`}>{body}</span>
  }

  return (
    <span className="relative inline-block">
      <button
        type="button"
        aria-describedby={open ? id : undefined}
        onClick={() => setOpen(!open)}
        onBlur={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        className={`${shape} ${tone}`}
      >
        {body}
      </button>
      {open && explain && (
        <span
          id={id}
          role="tooltip"
          className="absolute left-0 top-full z-20 mt-1 w-64 rounded border border-edge-strong bg-raised px-2.5 py-2 text-left text-xs font-normal normal-case leading-relaxed tracking-normal text-ink shadow-xl"
        >
          {explain}
        </span>
      )}
    </span>
  )
}

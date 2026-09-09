import { useId, useState } from 'react'

// This explanation used to live in a title attribute — invisible on touch,
// invisible to the keyboard, and it happened to hold the product's pitch.
export default function Chip({ label, tone, explain }) {
  const [open, setOpen] = useState(false)
  const id = useId()

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
        className={`rounded border px-1.5 py-px text-[11px] leading-4 ${tone}`}
      >
        {label}
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

import { useId } from 'react'

// A button that cannot be used yet, and says why beside it, in words. A greyed
// button alone leaves the person guessing what is missing, and a title
// attribute reaches neither the keyboard nor a phone.
//
// It stays focusable — aria-disabled rather than disabled — so Tab still finds
// it and the reason is read out with it, instead of the control silently
// vanishing from the page for anyone not looking at it.
//
// reasonId, when given, points at a reason already on screen: two buttons
// blocked for the same cause should not say it twice.
export default function Gated({ reason, reasonId, onClick, className = '', children }) {
  const id = useId()

  if (!reason && !reasonId) {
    return (
      <button onClick={onClick} className={className}>
        {children}
      </button>
    )
  }

  const button = (
    <button
      aria-disabled="true"
      aria-describedby={reasonId ?? id}
      onClick={(e) => e.preventDefault()}
      className={`${className} cursor-not-allowed opacity-40`}
    >
      {children}
    </button>
  )

  if (reasonId) return button

  return (
    <span className="inline-flex flex-wrap items-center justify-end gap-2">
      <span id={id} className="text-xs text-muted">
        {reason}
      </span>
      {button}
    </span>
  )
}

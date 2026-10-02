import { useEffect, useId, useRef } from 'react'

// A dialog that cannot be left with Escape, whose focus wanders onto a page
// still refreshing behind it, is a trap — and these are the windows opened
// when something is on fire, sometimes with one hand on a phone.
//
// Every dialog uses this one: focus moves in when it opens (unless a field
// already took it), Tab stays inside, Escape asks to close, and focus goes
// back to whatever opened it.
//
// onClose is read through a ref. Depending on it re-ran the effect on every
// render of the parent — every five seconds, on a page that polls — and each
// run handed focus back to the page and took it again, so a person in the
// middle of a field lost their place.
export function useDialog(onClose) {
  const frame = useRef(null)
  const close = useRef(onClose)
  const titleId = useId()
  close.current = onClose

  useEffect(() => {
    const restore = document.activeElement
    if (!frame.current?.contains(document.activeElement)) frame.current?.focus()

    function onKey(event) {
      if (event.key === 'Escape') {
        event.stopPropagation()
        return close.current?.()
      }
      if (event.key !== 'Tab' || !frame.current) return

      const reachable = frame.current.querySelectorAll(
        'a[href],button:not([disabled]),select:not([disabled]),input:not([disabled]),textarea,[tabindex]:not([tabindex="-1"])',
      )
      if (!reachable.length) return

      const first = reachable[0]
      const last = reachable[reachable.length - 1]

      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      restore?.focus?.()
    }
  }, [])

  return { frame, titleId }
}

// The attributes every dialog frame carries.
export function dialogProps({ frame, titleId }) {
  return { ref: frame, role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': titleId, tabIndex: -1 }
}

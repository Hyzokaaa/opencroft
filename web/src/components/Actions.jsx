import { useEffect, useId, useRef, useState } from 'react'

// The actions of a row, always on screen. Hidden until hovered they did not
// exist on a phone and were never found on a desktop; a quiet button that is
// there costs less than a loud one nobody knows about.
//
// Below the small breakpoint they fold into one menu, because a row of five
// buttons does not fit beside a name on a phone. The menu floats over the
// page — fixed to where its button is, so no card or table clips it — and
// closes on a tap outside, on Escape, or once something in it is chosen.
//
//   actions: [{ label, onClick, danger? }]
export default function Actions({ actions, label = 'Actions' }) {
  const shown = actions.filter(Boolean)
  const [at, setAt] = useState(null)
  const trigger = useRef(null)
  const menu = useRef(null)
  const id = useId()

  useEffect(() => {
    if (!at) return
    const outside = (e) => {
      if (!menu.current?.contains(e.target) && !trigger.current?.contains(e.target)) setAt(null)
    }
    const key = (e) => {
      if (e.key !== 'Escape') return
      setAt(null)
      trigger.current?.focus()
    }
    const away = () => setAt(null)
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', key)
    addEventListener('scroll', away, true)
    addEventListener('resize', away)
    menu.current?.querySelector('button')?.focus()
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('keydown', key)
      removeEventListener('scroll', away, true)
      removeEventListener('resize', away)
    }
  }, [at])

  if (!shown.length) return null

  const button = (a, extra = '') => (
    <button
      key={a.label}
      onClick={(e) => {
        e.stopPropagation()
        setAt(null)
        a.onClick()
      }}
      className={`rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong ${
        a.danger ? 'hover:border-problem/50 hover:text-problem' : 'hover:text-ink'
      } ${extra}`}
    >
      {a.label}
    </button>
  )

  function toggle(e) {
    e.stopPropagation()
    if (at) return setAt(null)
    const rect = trigger.current.getBoundingClientRect()
    const right = window.innerWidth - rect.right
    // Near the bottom of the screen it opens upwards, so it is never cut off.
    const tall = 40 * shown.length + 16
    setAt(rect.bottom + tall > window.innerHeight
      ? { bottom: window.innerHeight - rect.top + 4, right }
      : { top: rect.bottom + 4, right })
  }

  return (
    <>
      <div className="hidden flex-wrap justify-end gap-1.5 sm:flex">{shown.map((a) => button(a))}</div>
      <div className="flex justify-end sm:hidden">
        <button
          ref={trigger}
          onClick={toggle}
          aria-label={label}
          aria-haspopup="true"
          aria-expanded={Boolean(at)}
          aria-controls={id}
          className="flex size-8 items-center justify-center rounded border border-edge text-muted transition hover:text-ink"
        >
          &hellip;
        </button>
        {at && (
          <div
            ref={menu}
            id={id}
            onClick={(e) => e.stopPropagation()}
            // Leaving it with Tab closes it, as a click elsewhere does. Only
            // when focus really went somewhere: Safari on a phone gives a
            // tapped button no focus, and closing on that empty blur would
            // take the menu away before the tap's click arrives.
            onBlur={(e) =>
              e.relatedTarget && !menu.current?.contains(e.relatedTarget) && e.relatedTarget !== trigger.current && setAt(null)
            }
            style={{ position: 'fixed', top: at.top, bottom: at.bottom, right: at.right }}
            className="z-50 flex min-w-40 flex-col gap-1 rounded border border-edge-strong bg-raised p-1.5 shadow-2xl"
          >
            {shown.map((a) => button(a, 'w-full py-1.5 text-left'))}
          </div>
        )}
      </div>
    </>
  )
}

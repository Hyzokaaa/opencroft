// The actions of a row, always on screen. Hidden until hovered they did not
// exist on a phone and were never found on a desktop; a quiet button that is
// there costs less than a loud one nobody knows about.
//
// Below the small breakpoint they fold into one menu, because a row of five
// buttons does not fit beside a name on a phone.
//
//   actions: [{ label, onClick, danger? }]
export default function Actions({ actions, label = 'Actions' }) {
  const shown = actions.filter(Boolean)
  if (!shown.length) return null

  const button = (a, extra = '') => (
    <button
      key={a.label}
      onClick={(e) => {
        e.stopPropagation()
        e.currentTarget.closest('details')?.removeAttribute('open')
        a.onClick()
      }}
      className={`rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong ${
        a.danger ? 'hover:border-problem/50 hover:text-problem' : 'hover:text-ink'
      } ${extra}`}
    >
      {a.label}
    </button>
  )

  return (
    <>
      <div className="hidden flex-wrap justify-end gap-1.5 sm:flex">{shown.map((a) => button(a))}</div>
      <details className="relative sm:hidden" onClick={(e) => e.stopPropagation()}>
        <summary
          aria-label={label}
          className="ml-auto flex size-7 cursor-pointer list-none items-center justify-center rounded border border-edge text-muted [&::-webkit-details-marker]:hidden"
        >
          &hellip;
        </summary>
        <div className="mt-1 flex min-w-32 flex-col gap-1 rounded border border-edge bg-raised p-1.5">
          {shown.map((a) => button(a, 'w-full text-left'))}
        </div>
      </details>
    </>
  )
}

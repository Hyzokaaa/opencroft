import Command from './Command.jsx'

// description, when given, is one quiet line under the title: what the card
// is for, said once rather than beside every field in it.
export default function Card({ title, count, description, action, commands, commandMode, children, id }) {
  return (
    <section id={id} className="overflow-hidden rounded-lg border border-edge bg-panel">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-edge px-4 py-2.5">
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <h2 className="text-sm font-medium">{title}</h2>
            {count !== undefined && <span className="text-xs text-faint">{count}</span>}
          </div>
          {description && <p className="mt-0.5 text-xs text-muted">{description}</p>}
        </div>
        {action}
      </header>

      <div className="overflow-x-auto">{children}</div>

      {commandMode && commands && (
        <footer className="border-t border-edge px-4 py-2.5">
          <p className="mb-1.5 text-[11px] uppercase tracking-wide text-faint">Read with</p>
          <Command lines={commands} />
        </footer>
      )}
    </section>
  )
}

import Command from './Command.jsx'

export default function Card({ title, count, action, commands, commandMode, children }) {
  return (
    <section className="overflow-hidden rounded-lg border border-edge bg-panel">
      <header className="flex items-center justify-between gap-2 border-b border-edge px-4 py-2.5">
        <div className="flex items-baseline gap-2">
          <h2 className="text-sm font-medium">{title}</h2>
          {count !== undefined && <span className="text-xs text-faint">{count}</span>}
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

import { useState } from 'react'
import { SEVERITY, remediesFor } from '../lib/vocabulary.js'
import Command from './Command.jsx'
import Chevron from './Chevron.jsx'

// Problems and notices are different things. A hand-edited file is the product
// keeping its promise; listing it beside an outage says the opposite.
//
// onRemedy(remedy) performs the fix a remedy names; see remediesFor.
export default function Findings({ findings, instances, routes, commandMode, onFocus, onRemedy, checkedAt }) {
  const problems = findings.filter((f) => f.severity !== 'info')
  const notices = findings.filter((f) => f.severity === 'info')
  const context = { instances, routes }

  return (
    <div className="space-y-2">
      {/* The verdict above already says "All good"; this says what that
          claim rests on, which is what makes it believable. */}
      {problems.length === 0 && (
        <p className="px-1 text-xs text-muted">
          Checked {plural(instances.length, 'container')} and {plural(routes.length, 'domain')}
          {checkedAt ? ` ${checkedAt}` : ''}.
        </p>
      )}

      {problems.map((f) => (
        <Problem
          key={`${f.kind}:${f.subject}`}
          finding={f}
          remedies={remediesFor(f, context)}
          commandMode={commandMode}
          onFocus={onFocus}
          onRemedy={onRemedy}
        />
      ))}

      {notices.length > 0 && (
        <Notices notices={notices} context={context} onFocus={onFocus} onRemedy={onRemedy} />
      )}
    </div>
  )
}

const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`

function Problem({ finding, remedies, commandMode, onFocus, onRemedy }) {
  const tone = SEVERITY[finding.severity] ?? SEVERITY.warning
  const [showCommand, setShowCommand] = useState(false)

  return (
    <div className="flex overflow-hidden rounded-lg border border-edge bg-panel">
      <div className={`w-[3px] shrink-0 ${tone.accent}`} />
      <div className="min-w-0 flex-1 px-4 py-3">
        <div className="flex items-start gap-2.5">
          <span
            className={`mt-px flex size-4 shrink-0 items-center justify-center rounded-full border border-current font-mono text-[10px] font-bold ${tone.text}`}
            aria-hidden
          >
            {tone.icon}
          </span>
          <div className="min-w-0 flex-1">
            <p className="text-sm">
              <span className="sr-only">{tone.label}: </span>
              <button
                onClick={() => onFocus(finding.subject)}
                className="font-mono font-medium underline decoration-edge-strong underline-offset-4 hover:decoration-ink"
              >
                {finding.subject}
              </button>
              <span className="text-ink/90"> &mdash; {finding.message}</span>
            </p>
            {finding.hint && <p className="mt-1 text-xs text-muted">{finding.hint}</p>}

            {remedies.length > 0 && (
              <div className="mt-3">
                <div className="flex flex-wrap items-center gap-2">
                  {remedies.map((remedy, i) => (
                    <button
                      key={remedy.act}
                      onClick={() => onRemedy(remedy)}
                      className={`rounded border px-2.5 py-1 text-xs transition ${
                        i === 0
                          ? 'border-edge-strong bg-raised hover:border-ink/30'
                          : 'border-edge text-muted hover:border-edge-strong hover:text-ink'
                      }`}
                    >
                      {remedy.label}
                    </button>
                  ))}
                  {!commandMode && (
                    <button
                      onClick={() => setShowCommand(!showCommand)}
                      aria-expanded={showCommand}
                      className="font-mono text-xs text-faint transition hover:text-muted"
                    >
                      {showCommand ? 'hide command' : '$ show command'}
                    </button>
                  )}
                </div>
                {(showCommand || commandMode) && (
                  <Command lines={remedies.map((r) => r.command)} className="mt-2 max-w-lg" />
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// The positioning statement, delivered calmly, exactly where the user is
// already looking. More persuasive than a line of copy in the footer.
function Notices({ notices, context, onFocus, onRemedy }) {
  const [open, setOpen] = useState(false)

  return (
    <div className="rounded-lg border border-edge/60 px-4 py-2.5">
      <button
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 text-left text-xs text-muted transition hover:text-ink"
      >
        <Chevron open={open} />
        <span>
          {notices.length} found on the host, not created here &mdash; shown, and never changed
          unless you take {notices.length === 1 ? 'it' : 'them'} over.
        </span>
      </button>

      {open && (
        <ul className="mt-2.5 space-y-2.5 border-t border-edge pt-2.5">
          {notices.map((f) => {
            const remedies = remediesFor(f, context)
            return (
              <li key={`${f.kind}:${f.subject}`} className="flex gap-2.5 text-xs">
                <span className="mt-1 size-1.5 shrink-0 rounded-full bg-yours" />
                <div className="min-w-0 flex-1">
                  <button
                    onClick={() => onFocus(f.subject)}
                    className="font-mono text-ink underline decoration-edge-strong underline-offset-4 hover:decoration-ink"
                  >
                    {f.subject}
                  </button>
                  <span className="text-muted"> &mdash; {f.message}</span>
                  {f.hint && <p className="mt-0.5 text-faint">{f.hint}</p>}
                </div>
                {remedies.map((remedy) => (
                  <button
                    key={remedy.act}
                    onClick={() => onRemedy(remedy)}
                    className="h-fit shrink-0 rounded border border-edge px-2 py-0.5 text-xs text-muted transition hover:border-edge-strong hover:text-ink"
                  >
                    {remedy.label}&hellip;
                  </button>
                ))}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

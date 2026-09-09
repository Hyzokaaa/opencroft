import { useState } from 'react'
import { SEVERITY, remedyFor } from '../lib/vocabulary.js'
import Command from './Command.jsx'
import Chevron from './Chevron.jsx'

// Problems and notices are different things. A hand-edited file is the product
// keeping its promise; listing it beside an outage says the opposite.
export default function Findings({ findings, instances, routes, commandMode, onFocus, checkedAt }) {
  const problems = findings.filter((f) => f.severity !== 'info')
  const notices = findings.filter((f) => f.severity === 'info')

  return (
    <div className="space-y-2">
      {problems.length === 0 && (
        <AllClear instances={instances} routes={routes} checkedAt={checkedAt} />
      )}

      {problems.map((f) => (
        <Problem
          key={`${f.kind}:${f.subject}`}
          finding={f}
          instances={instances}
          commandMode={commandMode}
          onFocus={onFocus}
        />
      ))}

      {notices.length > 0 && <Notices notices={notices} onFocus={onFocus} />}
    </div>
  )
}

// Saying what was checked is what makes "nothing wrong" believable.
function AllClear({ instances, routes, checkedAt }) {
  const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`

  return (
    <div className="flex items-center gap-3 rounded-lg border border-edge bg-panel px-4 py-3">
      <span className="size-2 shrink-0 rounded-full bg-running" />
      <p className="text-sm text-muted">
        Checked {plural(instances.length, 'container')} and {plural(routes.length, 'domain')}
        {checkedAt ? ` ${checkedAt}` : ''}. Nothing needs your attention.
      </p>
    </div>
  )
}

function Problem({ finding, instances, commandMode, onFocus }) {
  const tone = SEVERITY[finding.severity] ?? SEVERITY.warning
  const remedy = remedyFor(finding, instances)
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
              <button
                onClick={() => onFocus(finding.subject)}
                className="font-mono font-medium underline decoration-edge-strong underline-offset-4 hover:decoration-ink"
              >
                {finding.subject}
              </button>
              <span className="text-ink/90"> &mdash; {finding.message}</span>
            </p>
            {finding.hint && <p className="mt-1 text-xs text-muted">{finding.hint}</p>}

            {remedy && (
              <div className="mt-3">
                <div className="flex flex-wrap items-center gap-2">
                  <button className="rounded border border-edge-strong bg-raised px-2.5 py-1 text-xs transition hover:border-ink/30">
                    {remedy.label}
                  </button>
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
                  <Command lines={[remedy.command]} className="mt-2 max-w-lg" />
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
function Notices({ notices, onFocus }) {
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
          {notices.length} resource{notices.length === 1 ? ' is' : 's are'} yours, not ours &mdash;
          listed, never touched.
        </span>
      </button>

      {open && (
        <ul className="mt-2.5 space-y-2.5 border-t border-edge pt-2.5">
          {notices.map((f) => (
            <li key={`${f.kind}:${f.subject}`} className="flex gap-2.5 text-xs">
              <span className="mt-1 size-1.5 shrink-0 rounded-full bg-yours" />
              <div>
                <button
                  onClick={() => onFocus(f.subject)}
                  className="font-mono text-ink underline decoration-edge-strong underline-offset-4 hover:decoration-ink"
                >
                  {f.subject}
                </button>
                <span className="text-muted"> &mdash; {f.message}</span>
                {f.hint && <p className="mt-0.5 text-faint">{f.hint}</p>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

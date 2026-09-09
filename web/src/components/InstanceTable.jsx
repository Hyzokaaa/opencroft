import Chip from './Chip.jsx'
import { OWNERSHIP } from '../lib/vocabulary.js'

// A stopped container is normal. A stopped container with a domain pointing at
// it is an incident — so the row inherits the severity of whatever finding
// names it. Two truths on one screen is worse than none.
export default function InstanceTable({ instances, problems, highlighted, onHover, onFocus, compact }) {
  if (!instances.length) {
    return <p className="px-4 py-8 text-center text-sm text-muted">No containers on this host yet.</p>
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
          <th className="px-4 py-2 font-medium">Container</th>
          <th className="px-4 py-2 font-medium">Address</th>
          {!compact && <th className="px-4 py-2 font-medium">Domain</th>}
          <th className="px-4 py-2 font-medium">Resources</th>
        </tr>
      </thead>
      <tbody>
        {instances.map((i) => {
          const problem = problems.has(i.name) || (i.domain && problems.has(i.domain))
          const lit = highlighted === i.name || (i.domain && highlighted === i.domain)

          return (
            <tr
              key={i.name}
              onMouseEnter={() => onHover(i.name)}
              onMouseLeave={() => onHover(null)}
              onClick={() => onFocus(i.name)}
              className={`cursor-pointer border-b border-edge/50 transition-colors last:border-0 ${
                lit ? 'bg-white/[0.05]' : 'hover:bg-white/[0.03]'
              }`}
            >
              <td className="relative px-4 py-2.5">
                {problem && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
                <div className="flex items-center gap-2">
                  <StatusDot status={i.status} problem={problem} />
                  <span className="font-mono font-medium">{i.name}</span>
                  {!i.managed && (
                    <Chip
                      label={OWNERSHIP.unmanaged.label}
                      tone={OWNERSHIP.unmanaged.tone}
                      explain={OWNERSHIP.unmanaged.explain}
                    />
                  )}
                </div>
                {i.image && <p className="mt-0.5 pl-[18px] text-xs text-faint">{i.image}</p>}
              </td>

              <td className="px-4 py-2.5 font-mono text-xs">
                {i.address || <span className="text-faint">&mdash;</span>}
                {i.port ? <span className="text-faint">:{i.port}</span> : null}
              </td>

              {!compact && (
                <td className="px-4 py-2.5 font-mono text-xs">
                  {i.domain ? (
                    <>
                      {i.domain}
                      {/* One container commonly answers on several names. */}
                      {i.domains?.length > 1 && (
                        <span className="text-faint" title={i.domains.join('\n')}>
                          {' '}+{i.domains.length - 1}
                        </span>
                      )}
                    </>
                  ) : (
                    <span className="text-faint">&mdash;</span>
                  )}
                </td>
              )}

              <td className="whitespace-nowrap px-4 py-2.5 text-xs text-muted">
                {i.cpuLimit ? `${i.cpuLimit} CPU` : '—'}
                {i.memLimit ? ` · ${i.memLimit}` : ''}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// Never colour alone: a stopped container also loses its fill, and one in
// trouble carries a ring.
function StatusDot({ status, problem }) {
  if (problem) {
    return <span className="size-2.5 shrink-0 rounded-full bg-problem ring-2 ring-problem/25" title="Needs attention" />
  }
  if (status === 'running') {
    return <span className="size-2.5 shrink-0 rounded-full bg-running" title="running" />
  }
  return (
    <span
      className="size-2.5 shrink-0 rounded-full border border-stopped bg-transparent"
      title={status}
    />
  )
}

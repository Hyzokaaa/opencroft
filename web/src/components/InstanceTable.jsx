import Chip from './Chip.jsx'
import Actions from './Actions.jsx'
import { OWNERSHIP, VERBS } from '../lib/vocabulary.js'
import { href } from '../lib/useRoute.js'

// A stopped container is normal. A stopped container with a domain pointing at
// it is an incident — so the row inherits the severity of whatever finding
// names it. Two truths on one screen is worse than none.
export default function InstanceTable({ instances, problems, highlighted, onHover, onDestroy, onAddDomain, onPower, onDeploy, empty }) {
  if (!instances.length) {
    return <p className="px-4 py-8 text-center text-sm text-muted">{empty ?? 'No containers on this host yet.'}</p>
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
            <th className="px-4 py-2 font-medium">Container</th>
            <th className="hidden px-4 py-2 font-medium md:table-cell">Project</th>
            <th className="hidden px-4 py-2 font-medium sm:table-cell">Address</th>
            <th className="hidden px-4 py-2 font-medium lg:table-cell">Domain</th>
            <th className="hidden px-4 py-2 font-medium lg:table-cell">Resources</th>
            <th className="px-4 py-2"><span className="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {instances.map((i) => {
            const problem = problems.has(i.name) || (i.domain && problems.has(i.domain))
            const lit = highlighted === i.name || (i.domain && highlighted === i.domain)

            return (
              <tr
                key={i.name}
                onMouseEnter={() => onHover?.(i.name)}
                onMouseLeave={() => onHover?.(null)}
                // The name is the link; the row is a larger target for a mouse.
                onClick={() => (location.hash = href('containers', i.name))}
                className={`cursor-pointer border-b border-edge/50 transition-colors last:border-0 ${
                  lit ? 'bg-white/[0.05]' : 'hover:bg-white/[0.03]'
                }`}
              >
                <td className="relative px-4 py-2.5">
                  {problem && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
                  <div className="flex items-center gap-2">
                    <StatusDot status={i.status} problem={problem} />
                    <a
                      href={href('containers', i.name)}
                      onClick={(e) => e.stopPropagation()}
                      className="font-mono font-medium underline-offset-4 hover:underline"
                    >
                      {i.name}
                    </a>
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

                <td className="hidden px-4 py-2.5 text-xs md:table-cell">
                  {i.project ? (
                    <a
                      href={href('projects', i.project)}
                      onClick={(e) => e.stopPropagation()}
                      className="font-mono text-muted underline-offset-4 hover:text-ink hover:underline"
                    >
                      {i.project}
                    </a>
                  ) : (
                    <span className="text-faint">&mdash;</span>
                  )}
                </td>

                <td className="hidden px-4 py-2.5 font-mono text-xs sm:table-cell">
                  {i.address || <span className="text-faint">&mdash;</span>}
                  {i.port ? <span className="text-faint">:{i.port}</span> : null}
                </td>

                <td className="hidden px-4 py-2.5 font-mono text-xs lg:table-cell">
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

                <td className="hidden whitespace-nowrap px-4 py-2.5 text-xs text-muted lg:table-cell">
                  {i.cpuLimit ? `${i.cpuLimit} CPU` : "—"}
                  {i.memLimit ? ` · ${i.memLimit}` : ""}
                </td>

                <td className="px-4 py-2.5 text-right">
                  <Actions
                    label={`Actions for ${i.name}`}
                    actions={[
                      { label: i.status === 'running' ? 'Stop' : 'Start', onClick: () => onPower?.(i, i.status === 'running') },
                      { label: 'Deploy', onClick: () => onDeploy?.(i) },
                      { label: 'Add domain', onClick: () => onAddDomain?.(i) },
                      // Destroying something croft did not create is allowed,
                      // but it is the user's call and the plan says so plainly.
                      { label: `${VERBS.destroy}…`, onClick: () => onDestroy?.(i.name), danger: true },
                    ]}
                  />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// Never colour alone: a stopped container also loses its fill, one in trouble
// carries a ring, and a screen reader hears the word.
export function StatusDot({ status, problem }) {
  const word = problem ? 'needs attention' : status
  const shape = problem
    ? 'bg-problem ring-2 ring-problem/25'
    : status === 'running'
      ? 'bg-running'
      : 'border border-stopped bg-transparent'

  return (
    <>
      <span className={`size-2.5 shrink-0 rounded-full ${shape}`} title={word} aria-hidden="true" />
      <span className="sr-only">{word}</span>
    </>
  )
}

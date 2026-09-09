import Chip from './Chip.jsx'
import { OWNERSHIP } from '../lib/vocabulary.js'

export default function RouteTable({ routes, instances, problems, highlighted, onHover, onFocus, compact }) {
  if (!routes.length) {
    return <p className="px-4 py-8 text-center text-sm text-muted">No domains routed yet.</p>
  }

  const nameFor = (address) => instances.find((i) => i.address === address)?.name

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
          <th className="px-4 py-2 font-medium">Domain</th>
          <th className="px-4 py-2 font-medium">Serves</th>
          {!compact && <th className="px-4 py-2 font-medium">File</th>}
        </tr>
      </thead>
      <tbody>
        {routes.map((r) => {
          const target = nameFor(r.target)
          const ownership = OWNERSHIP[r.state] ?? OWNERSHIP.managed
          const problem = problems.has(r.domain)
          const lit = highlighted === r.domain || (target && highlighted === target)

          return (
            <tr
              key={r.domain}
              onMouseEnter={() => onHover(target ?? r.domain)}
              onMouseLeave={() => onHover(null)}
              onClick={() => onFocus(r.domain)}
              className={`cursor-pointer border-b border-edge/50 transition-colors last:border-0 ${
                lit ? 'bg-white/[0.05]' : 'hover:bg-white/[0.03]'
              }`}
            >
              <td className="relative px-4 py-2.5">
                {problem && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
                <div className="flex items-center gap-2">
                  {r.ssl ? <Lock /> : <span className="w-3 shrink-0" />}
                  <span className="font-mono">{r.domain}</span>
                  {r.state !== 'managed' && (
                    <Chip label={ownership.label} tone={ownership.tone} explain={ownership.explain} />
                  )}
                </div>
              </td>

              <td className="px-4 py-2.5 text-xs">
                <span className="font-mono">{target ?? r.target}</span>
                <span className="text-faint">:{r.port}</span>
              </td>

              {!compact && (
                <td className="px-4 py-2.5 font-mono text-xs text-faint">{r.file}</td>
              )}
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// TLS is a property, not a state — it gets an icon, not the green that means
// "running".
function Lock() {
  return (
    <svg viewBox="0 0 16 16" className="size-3 shrink-0 text-muted" fill="currentColor" aria-label="TLS">
      <path d="M8 1a3 3 0 0 0-3 3v2H4.5A1.5 1.5 0 0 0 3 7.5v6A1.5 1.5 0 0 0 4.5 15h7a1.5 1.5 0 0 0 1.5-1.5v-6A1.5 1.5 0 0 0 11.5 6H11V4a3 3 0 0 0-3-3Zm0 1.5A1.5 1.5 0 0 1 9.5 4v2h-3V4A1.5 1.5 0 0 1 8 2.5Z" />
    </svg>
  )
}

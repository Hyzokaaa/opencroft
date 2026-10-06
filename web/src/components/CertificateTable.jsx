import Chip from './Chip.jsx'
import { renewedBy } from '../lib/domains.js'

// Certificates are the one thing on this panel with a deadline. The column
// that matters is not who issued it — it is how long you have, so the soonest
// to expire comes first, and who renews it sits beside it: one nobody renews
// is the one that will expire.
export default function CertificateTable({ certificates }) {
  if (!certificates.length) {
    return (
      <div className="px-4 py-8 text-center">
        <p className="text-sm text-muted">No certificates on this host.</p>
        <p className="mt-1 text-xs text-faint">
          Read from the certificate files themselves, wherever they came from.
        </p>
      </div>
    )
  }

  const sorted = [...certificates].sort((a, b) => a.daysLeft - b.daysLeft)

  return (
    <div className="overflow-x-auto">
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
          <th className="px-4 py-2 font-medium">Domain</th>
          <th className="px-4 py-2 font-medium">Expires</th>
          <th className="px-4 py-2 font-medium">Renewed by</th>
          <th className="hidden px-4 py-2 font-medium sm:table-cell">Issuer</th>
          <th className="hidden px-4 py-2 font-medium lg:table-cell">File</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((c) => {
          // One certificate commonly answers for several names, and which
          // ones is the question asked of it: all of them, said.
          const others = (c.names ?? []).filter((n) => n !== c.domain)
          const renewer = renewedBy(c)
          return (
          <tr key={c.domain} className="border-b border-edge/50 last:border-0">
            <td className="relative px-4 py-2.5">
              {c.daysLeft < 21 && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
              <div className="flex flex-wrap items-center gap-2">
                <span className="break-all font-mono">{c.domain}</span>
                {c.selfSigned && (
                  <Chip
                    label="self-signed"
                    tone="border-caution/40 text-caution"
                    explain="This certificate signed itself. Browsers will warn about it — fine for a private service, not for a public one."
                  />
                )}
              </div>
              {others.length > 0 && (
                <p className="mt-0.5 break-all font-mono text-[11px] text-faint">
                  <span className="font-sans">also </span>{others.join(', ')}
                </p>
              )}
            </td>

            <td className="whitespace-nowrap px-4 py-2.5">
              <Remaining days={c.daysLeft} />
            </td>

            <td className="px-4 py-2.5 text-xs">
              {renewer ? <span className="text-muted">{renewer}</span> : <span className="text-caution">nobody croft knows of</span>}
            </td>
            <td className="hidden px-4 py-2.5 text-xs text-muted sm:table-cell">{c.issuer || '—'}</td>
            <td className="hidden px-4 py-2.5 font-mono text-xs text-faint lg:table-cell">{c.path}</td>
          </tr>
          )
        })}
      </tbody>
    </table>
    </div>
  )
}

function Remaining({ days }) {
  if (days < 0) {
    return <span className="text-problem">expired {Math.abs(days)} days ago</span>
  }
  if (days < 7) {
    return <span className="text-problem">in {days} days</span>
  }
  if (days < 21) {
    return <span className="text-caution">in {days} days</span>
  }
  return <span className="text-muted">in {days} days</span>
}

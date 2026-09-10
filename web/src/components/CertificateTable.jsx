import Chip from './Chip.jsx'

// Certificates are the one thing on this panel with a deadline. The column
// that matters is not who issued it — it is how long you have.
export default function CertificateTable({ certificates, onFocus }) {
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

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-edge text-left text-[11px] uppercase tracking-wide text-faint">
          <th className="px-4 py-2 font-medium">Domain</th>
          <th className="px-4 py-2 font-medium">Expires</th>
          <th className="px-4 py-2 font-medium">Issuer</th>
          <th className="px-4 py-2 font-medium">File</th>
        </tr>
      </thead>
      <tbody>
        {certificates.map((c) => (
          <tr
            key={c.domain}
            onClick={() => onFocus?.(c.domain)}
            className="cursor-pointer border-b border-edge/50 transition-colors last:border-0 hover:bg-white/[0.03]"
          >
            <td className="relative px-4 py-2.5">
              {c.daysLeft < 21 && <span className="absolute left-0 top-0 h-full w-[2px] bg-problem" />}
              <div className="flex items-center gap-2">
                <span className="font-mono">{c.domain}</span>
                {c.selfSigned && (
                  <Chip
                    label="self-signed"
                    tone="border-caution/40 text-caution"
                    explain="This certificate signed itself. Browsers will warn about it — fine for a private service, not for a public one."
                  />
                )}
                {/* One certificate commonly answers for several names. */}
                {c.names?.length > 1 && (
                  <span className="text-[11px] text-faint">+{c.names.length - 1} more</span>
                )}
              </div>
            </td>

            <td className="whitespace-nowrap px-4 py-2.5">
              <Remaining days={c.daysLeft} />
            </td>

            <td className="px-4 py-2.5 text-xs text-muted">{c.issuer || '—'}</td>
            <td className="px-4 py-2.5 font-mono text-xs text-faint">{c.path}</td>
          </tr>
        ))}
      </tbody>
    </table>
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

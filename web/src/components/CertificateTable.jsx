import Chip from './Chip.jsx'
import { PageHead } from './Listing.jsx'
import { renewedBy } from '../lib/domains.js'
import { PRIMARY } from '../lib/ui.js'

// Certificates are the one thing on this panel with a deadline. The column
// that matters is not who issued it — it is how long you have, so the soonest
// to expire comes first, and who renews it sits beside it: one nobody renews
// is the one that will expire.
// renewable, when given, is the route croft could take a certificate's renewal
// over through — one it wrote and serves on https — or nothing.
export default function CertificateTable({ certificates, renewable, onRenew }) {
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
    {/* Fixed widths, so the two groups on the page line up as one list. */}
    <table className="w-full text-sm lg:table-fixed">
      <thead>
        <tr className="border-b border-edge text-left text-xs text-muted">
          <th className="px-4 py-2.5 font-normal lg:w-[30%]">Domain</th>
          <th className="px-4 py-2.5 font-normal lg:w-[16%]">Expires</th>
          <th className="px-4 py-2.5 font-normal lg:w-[16%]">Renewed by</th>
          <th className="hidden px-4 py-2.5 font-normal sm:table-cell lg:w-[14%]">Issuer</th>
          <th className="hidden px-4 py-2.5 font-normal lg:table-cell">File</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((c) => {
          // One certificate commonly answers for several names, and which
          // ones is the question asked of it: all of them, said.
          const others = (c.names ?? []).filter((n) => n !== c.domain)
          const renewer = renewedBy(c)
          const takeOver = renewer !== 'croft' && !c.selfSigned ? renewable?.(c) : null
          return (
          <tr key={c.domain} className="border-b border-edge/60 last:border-0">
            <td className="relative px-4 py-3">
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

            <td className="whitespace-nowrap px-4 py-3">
              <Remaining days={c.daysLeft} />
            </td>

            <td className="px-4 py-3 text-xs">
              {renewer ? <span className="text-muted">{renewer}</span> : <span className="text-caution">nobody croft knows of</span>}
              {/* Offered only where croft wrote the vhost: there it can issue
                  its own and point the site at it, with a plan to read first.
                  Two programs renewing one name is how renewal breaks
                  quietly, so this replaces the other rather than joining it. */}
              {takeOver && (
                <button
                  onClick={() => onRenew(takeOver)}
                  className="mt-1 block text-left text-muted underline underline-offset-4 transition hover:text-ink"
                >
                  Renew with croft&hellip;
                </button>
              )}
            </td>
            <td className="hidden px-4 py-3 text-xs text-muted sm:table-cell">{c.issuer || '—'}</td>
            <td className="hidden px-4 py-3 break-all font-mono text-xs text-faint lg:table-cell">{c.path}</td>
          </tr>
          )
        })}
      </tbody>
    </table>
    </div>
  )
}

// The bar is the 90 days a Let's Encrypt certificate lives, so how much of it
// is left is seen before the number is read. The words stay beside it.
function Remaining({ days }) {
  const [text, bar] = days < 7 ? ['text-problem', 'bg-problem'] : days < 21 ? ['text-caution', 'bg-caution'] : ['text-muted', 'bg-muted/60']
  return (
    <>
      <span className={text}>{days < 0 ? `expired ${Math.abs(days)} days ago` : `in ${days} days`}</span>
      <span aria-hidden="true" className="mt-1 block h-1 w-28 overflow-hidden rounded bg-edge">
        <span
          className={`block h-full ${bar}`}
          style={{ width: `${Math.max(0, Math.min(100, (days / 90) * 100))}%` }}
        />
      </span>
    </>
  )
}

// CertificatesPage is every certificate on the host, those expiring within
// 21 days apart and first: that group is the reason to open the page.
export function CertificatesPage({ certificates, onWildcard, renewable, onRenew }) {
  const soon = certificates.filter((c) => c.daysLeft < 21)
  const rest = certificates.filter((c) => c.daysLeft >= 21)
  const nobody = certificates.filter((c) => !renewedBy(c) && !c.selfSigned).length

  return (
    <>
      <PageHead
        title="Certificates"
        summary={
          <>
            {certificates.length} certificate{certificates.length === 1 ? '' : 's'}
            {soon.length > 0 && <span className="text-caution"> · {soon.length} expire{soon.length === 1 ? 's' : ''} within 21 days</span>}
            {nobody > 0 && <span className="text-caution"> · {nobody} renewed by nobody croft knows of</span>}
            {' '}· read from the certificate files themselves, wherever they came from
          </>
        }
      >
        <button onClick={onWildcard} className={PRIMARY}>
          <span aria-hidden="true">+</span> Wildcard certificate
        </button>
      </PageHead>

      {certificates.length === 0 ? (
        <div className="rounded-lg border border-edge bg-panel">
          <CertificateTable certificates={[]} />
        </div>
      ) : (
        <>
          {soon.length > 0 && (
            <section className="space-y-2">
              <h2 className="text-xs font-medium text-caution">
                Expire within 21 days <span className="text-faint">{soon.length}</span>
              </h2>
              <div className="overflow-hidden rounded-lg border border-edge bg-panel">
                <CertificateTable certificates={soon} renewable={renewable} onRenew={onRenew} />
              </div>
            </section>
          )}
          {rest.length > 0 && (
            <section className="space-y-2">
              {soon.length > 0 && (
                <h2 className="text-xs font-medium text-muted">
                  The rest <span className="text-faint">{rest.length}</span>
                </h2>
              )}
              <div className="overflow-hidden rounded-lg border border-edge bg-panel">
                <CertificateTable certificates={rest} renewable={renewable} onRenew={onRenew} />
              </div>
            </section>
          )}
        </>
      )}
    </>
  )
}

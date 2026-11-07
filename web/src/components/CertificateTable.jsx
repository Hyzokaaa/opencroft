import Chip from './Chip.jsx'
import Command from './Command.jsx'
import { PageHead } from './Listing.jsx'
import { expiring, renewedBy } from '../lib/domains.js'
import { PRIMARY } from '../lib/ui.js'

// Certificates are the one thing on this panel with a deadline. The column
// that matters is not who issued it — it is how long you have, so the soonest
// to expire comes first, and who renews it sits beside it: one nobody renews
// is the one that will expire.
// renewable, when given, is the route a certificate is served on, or nothing.
export default function CertificateTable({ certificates, renewable, onRenew, onTakeOver }) {
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
          // The site served with it, when croft is not already renewing it.
          const site = renewer !== 'croft' && !c.unused ? renewable?.(c) : null
          const fix = !site ? null
            : site.state === 'managed' && !c.selfSigned ? { label: 'Renew with croft', run: () => onRenew(site) }
            : site.state === 'unmanaged' ? { label: 'Take it over to renew it', run: () => onTakeOver?.(site.domain) }
            : null
          return (
          <tr key={c.domain} className="border-b border-edge/60 last:border-0">
            <td className="relative px-4 py-3">
              {c.daysLeft < 21 && !c.unused && <span className={`absolute left-0 top-0 h-full w-[2px] ${c.daysLeft < 7 ? 'bg-problem' : 'bg-caution'}`} />}
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono [overflow-wrap:anywhere]">{c.domain}</span>
                {c.unused && (
                  <Chip
                    label="not used by any site"
                    tone="border-edge-strong text-muted"
                    explain="No site on this host is served with it: its files are still on disk, and its date matters to nobody."
                  />
                )}
                {c.selfSigned && (
                  <Chip
                    label="self-signed"
                    tone="border-caution/40 text-caution"
                    explain="This certificate signed itself. Browsers will warn about it — fine for a private service, not for a public one."
                  />
                )}
              </div>
              {others.length > 0 && (
                <p className="mt-0.5 font-mono [overflow-wrap:anywhere] text-[11px] text-faint">
                  <span className="font-sans">also </span>{others.join(', ')}
                </p>
              )}
            </td>

            <td className="whitespace-nowrap px-4 py-3">
              <Remaining days={c.daysLeft} quiet={c.unused} />
            </td>

            <td className="px-4 py-3 text-xs">
              {renewer ? <span className="text-muted">{renewer}</span> : <span className="text-caution">nobody croft knows of</span>}
              {/* The same fix Home offers for the same warning. Where croft
                  wrote the vhost it issues its own and points the site at it;
                  where it did not, the site is taken over first. Two programs
                  renewing one name is how renewal breaks quietly, so either
                  replaces the other rather than joining it. */}
              {fix && (
                <button
                  onClick={fix.run}
                  className="mt-1 block text-left text-muted underline underline-offset-4 transition hover:text-ink"
                >
                  {fix.label}&hellip;
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
function Remaining({ days, quiet }) {
  const [text, bar] = quiet ? ['text-faint', 'bg-edge-strong'] : days < 7 ? ['text-problem', 'bg-problem'] : days < 21 ? ['text-caution', 'bg-caution'] : ['text-muted', 'bg-muted/60']
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
// 21 days apart and first: that group is the reason to open the page. Those
// no site is served with go last, apart: left on disk, they expire without
// it mattering, and counting them hid the real deadline behind a false one.
export function CertificatesPage({ certificates, onWildcard, renewable, onRenew, onTakeOver }) {
  const used = certificates.filter((c) => !c.unused)
  const soon = expiring(certificates)
  const rest = used.filter((c) => c.daysLeft >= 21)
  const unused = certificates.filter((c) => c.unused)
  const nobody = used.filter((c) => !renewedBy(c) && !c.selfSigned).length

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
                <CertificateTable certificates={soon} renewable={renewable} onRenew={onRenew} onTakeOver={onTakeOver} />
              </div>
            </section>
          )}
          {rest.length > 0 && (
            <section className="space-y-2">
              {(soon.length > 0 || unused.length > 0) && (
                <h2 className="text-xs font-medium text-muted">
                  {soon.length > 0 ? 'The rest' : 'Served'} <span className="text-faint">{rest.length}</span>
                </h2>
              )}
              <div className="overflow-hidden rounded-lg border border-edge bg-panel">
                <CertificateTable certificates={rest} renewable={renewable} onRenew={onRenew} onTakeOver={onTakeOver} />
              </div>
            </section>
          )}
          {unused.length > 0 && (
            <section className="space-y-2">
              <h2 className="text-xs font-medium text-muted">
                Not used by any site <span className="text-faint">{unused.length}</span>
              </h2>
              {/* croft never deletes what it did not make. Saying how is as far
                  as it goes, and only for certbot's, whose own command keeps
                  its records straight. */}
              <p className="text-xs text-muted">
                No site on this host is served with these, so when they expire nothing changes. They are left where
                they are.
                {unused.some((c) => renewedBy(c) === 'certbot') && <> To remove certbot&apos;s, on the host:</>}
              </p>
              {unused.some((c) => renewedBy(c) === 'certbot') && (
                <Command
                  lines={unused
                    .filter((c) => renewedBy(c) === 'certbot')
                    .map((c) => `sudo certbot delete --cert-name ${c.path.split('/')[4]}`)}
                />
              )}
              <div className="overflow-hidden rounded-lg border border-edge bg-panel">
                <CertificateTable certificates={unused} />
              </div>
            </section>
          )}
        </>
      )}
    </>
  )
}

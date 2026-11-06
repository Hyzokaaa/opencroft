import { useMemo, useState } from 'react'
import Gated from './Gated.jsx'
import RouteTable from './RouteTable.jsx'
import { PageHead, Toolbar, matches } from './Listing.jsx'
import { useResources } from './Resources.jsx'
import { PRIMARY } from '../lib/ui.js'
import { serviceHref } from '../lib/useRoute.js'
import { allNames, answerOf, certificateFor, portOf } from '../lib/domains.js'

// Every name nginx serves on this host, and what a visitor typing it gets.
// Answers is asked of the container itself, by the name, so it tests this
// machine and not the DNS or a proxy in front of it — those are what the
// certificate and DNS columns are for.
export default function DomainsPage({ data, publicAddress, query, tableProps, onAddDomain }) {
  const resources = useResources()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState(query.tls ? 'tls' : 'all')
  const certificates = data.certificates ?? []

  // What listens where, so a row says "api in shop-api:3000", not a port.
  const services = useMemo(() => {
    const by = new Map()
    for (const c of resources.containers ?? []) {
      const container = data.instances.find((i) => i.name === c.name) ?? c
      for (const s of c.services?.services ?? []) {
        const key = `${container.address}:${portOf(s, container)}`
        if (!by.has(key)) by.set(key, { name: s.name, to: serviceHref(c.name, s.name) })
      }
    }
    return by
  }, [resources.containers, data.instances])
  const serviceAt = (address, port) => services.get(`${address}:${port}`)

  const names = allNames(data.routes)
  const attention = (n) => {
    const answer = answerOf(n)
    const covering = n.ssl ? certificateFor(n.domain, certificates) : null
    return answer.tone === 'problem' || answer.tone === 'caution' || (covering && covering.daysLeft < 21) || tableProps.problems.has(n.domain)
  }
  const byHand = (n) => n.route.state !== 'managed'
  const failing = names.filter((n) => answerOf(n).tone === 'problem').length
  const expiring = certificates.filter((c) => c.daysLeft < 21).length

  const filters = [
    { id: 'all', label: 'All', count: names.length },
    { id: 'attention', label: 'Needs attention', count: names.filter(attention).length },
    { id: 'hand', label: 'Not written by croft', count: names.filter(byHand).length },
    // Kept for the link from the home page that counts names on https.
    query.tls && { id: 'tls', label: 'On https', count: names.filter((n) => n.ssl).length },
  ].filter(Boolean)
  const kept = {
    all: () => true,
    attention,
    hand: byHand,
    tls: (n) => n.ssl,
  }[filter]
  const only = (n) =>
    kept(n) && matches(search, [n.domain, n.route.target, n.route.file, serviceAt(n.route.target, n.route.port)?.name])

  return (
    <>
      <PageHead
        title="Domains"
        summary={
          <>
            {names.length} name{names.length === 1 ? '' : 's'}
            {failing > 0 && <span className="text-problem"> · {failing} not serving</span>}
            {expiring > 0 && (
              <span className="text-caution"> · {expiring} certificate{expiring === 1 ? '' : 's'} expire{expiring === 1 ? 's' : ''} within 21 days</span>
            )}
            {publicAddress && <> · this host is at <span className="font-mono">{publicAddress}</span></>}
          </>
        }
      >
        {/* A domain is served from a container; with none, the form would
            offer an empty list to choose from. */}
        <Gated
          reason={data.instances.length ? undefined : 'Create a container first.'}
          onClick={() => onAddDomain(null)}
          className={PRIMARY}
        >
          <span aria-hidden="true">+</span> Add domain
        </Gated>
      </PageHead>

      <Toolbar
        query={search}
        onQuery={setSearch}
        placeholder="Search domains or what they reach"
        filters={filters}
        filter={filter}
        onFilter={setFilter}
      />

      <div className="overflow-hidden rounded-lg border border-edge bg-panel">
        <RouteTable {...tableProps} routes={data.routes} serviceAt={serviceAt} only={only} />
      </div>
    </>
  )
}

// What reaches what. A route is a vhost — one domain, maybe more names, maybe
// some paths sent elsewhere — but a person thinks in names: "is
// help.customer.com working?". These read the routes the overview carries and
// answer in names, so every page that lists domains says the same thing about
// each one.

// namesOf is every name a route answers on: its own first, then the others
// set up with it. Each keeps its route, which is where any action on it goes.
export function namesOf(route) {
  return [
    { key: route.domain, domain: route.domain, ssl: route.ssl, port: route.port, route, entry: true },
    ...(route.aliases ?? []).map((alias) => ({
      key: alias.domain, domain: alias.domain, ssl: alias.ssl, port: route.port, route, alias,
    })),
  ]
}

// allNames is every name on the host, one per row — what the Domains page
// lists and counts.
export function allNames(routes) {
  return routes.flatMap(namesOf)
}

// portOf is where a service listens: its own port, or — for a site the
// container's web server serves — the container's. The daemon decides the
// same way when a domain is given to a service.
export function portOf(service, container) {
  return service.port || container.port || 0
}

// reaching is every name and every path that leads to address:port — a
// service, once its container and port are known.
export function reaching(routes, address, port) {
  if (!address || !port) return []
  return routes.flatMap((r) => [
    ...(r.target === address && r.port === port ? namesOf(r) : []),
    ...(r.paths ?? [])
      .filter((p) => p.target === address && p.port === port)
      .map((p) => ({ key: r.domain + p.prefix, domain: r.domain + p.prefix, ssl: r.ssl, route: r, prefix: p.prefix, port: p.port })),
  ])
}

// byService sorts what reaches a container by the service listening where it
// leads. What leads to a port no service croft deployed listens on is set
// apart, with the port, because that is the domain a visitor gets a 502 from.
export function byService(container, services, routes) {
  // A name is listed once. One that leads to a port a service names as its
  // own goes to that service before one that only borrows the container's.
  const claimed = new Set()
  const take = (names) => names.filter((n) => !claimed.has(n.key) && claimed.add(n.key))
  const own = new Map(
    services.filter((s) => s.port).map((s) => [s.name, take(reaching(routes, container.address, s.port))]),
  )
  const groups = services.map((service) => ({
    service,
    port: portOf(service, container),
    names: own.get(service.name) ?? take(reaching(routes, container.address, portOf(service, container))),
  }))
  const stray = reachingContainer(routes, container.address).filter((n) => !claimed.has(n.key))
  return { groups, stray }
}

// reachingContainer is every name and path leading to the address, on any
// port, each with the port it leads to.
export function reachingContainer(routes, address) {
  if (!address) return []
  return routes.flatMap((r) => [
    ...(r.target === address ? namesOf(r).map((n) => ({ ...n, port: r.port })) : []),
    ...(r.paths ?? [])
      .filter((p) => p.target === address)
      .map((p) => ({ key: r.domain + p.prefix, domain: r.domain + p.prefix, ssl: r.ssl, route: r, prefix: p.prefix, port: p.port })),
  ])
}

// stateOf is how one name is served, in a word for the table and a sentence
// for whoever needs the reason. tone is a text class; the word is always
// there too, so colour never carries it alone.
//
//   down      the route answers nothing: nginx returns 502
//   waiting   croft is waiting for its DNS, and says why
//   http      served over http only — by choice, or until a certificate comes
//   https     served over https
//   found / edited  a vhost croft did not write, or one edited by hand
export function stateOf(name, certificates = []) {
  const { route, alias } = name
  if (route.answers === false) {
    return { key: 'down', label: 'nothing listening', tone: 'text-problem',
      reason: `Nothing accepts a connection on ${route.target}:${route.port}, so visitors get an error page.` }
  }
  if (alias?.waiting) {
    return { key: 'waiting', label: 'waiting for DNS', tone: 'text-caution', reason: alias.waiting }
  }
  if (!name.ssl) {
    return route.ssl
      ? { key: 'http', label: 'http only', tone: 'text-caution', reason: 'Waiting for its certificate. Until then it answers over http.' }
      : { key: 'http', label: 'http only', tone: 'text-muted' }
  }
  const certificate = certificateFor(name.domain, certificates)
  if (certificate && certificate.daysLeft < 21) {
    return {
      key: 'https', tone: certificate.daysLeft < 7 ? 'text-problem' : 'text-caution',
      label: certificate.daysLeft < 0 ? 'certificate expired' : `https · ${certificate.daysLeft} days left`,
    }
  }
  return { key: 'https', label: 'https', tone: 'text-muted' }
}

// certificateFor is the certificate a name is served with, if one on the host
// covers it: its own, one listing it, or a wildcard one label above.
export function certificateFor(domain, certificates = []) {
  return certificates.find(
    (c) => c.domain === domain || (c.names ?? []).some((n) => n === domain || covers(n, domain)) || covers(c.domain, domain),
  )
}

export function covers(name, domain) {
  if (!name?.startsWith('*.')) return false
  const base = name.slice(2)
  const label = domain.slice(0, -(base.length + 1))
  return domain.endsWith('.' + base) && label !== '' && !label.includes('.')
}

// renewedBy is who keeps a certificate alive: croft for its own directory,
// certbot for its, and nobody croft knows of otherwise — which is the one to
// worry about before it expires.
export function renewedBy(certificate) {
  if (certificate.selfSigned) return null
  if (certificate.path?.startsWith('/etc/letsencrypt/')) return 'certbot'
  if (certificate.managed) return 'croft'
  return null
}

// publicAddress is the address this host is reached at from outside, when the
// panel knows it — what a DNS record has to point at.
export function publicAddress(host) {
  return (host?.addresses ?? []).find((a) => a.public)?.address ?? null
}

// One word per concept, the same in every table.
// "not ours" is gone: from where the user sits, the whole server is theirs.
export const OWNERSHIP = {
  managed: {
    label: 'managed',
    tone: 'border-edge text-faint',
    explain: 'Created here and unchanged since. Safe to regenerate.',
  },
  adopted: {
    label: 'edited by hand',
    tone: 'border-yours/40 text-yours',
    explain: 'You edited this after it was generated. It stays exactly as you wrote it.',
  },
  unmanaged: {
    label: 'found here',
    tone: 'border-edge-strong text-muted',
    explain: 'Found on the host, not created here. Shown, and never changed unless you adopt or take it over.',
  },
}

// The verbs, once. The button, the dialog it opens and the history entry it
// leaves all say the same word, so that what was asked for can be found again.
export const VERBS = {
  adopt: 'Adopt',
  release: 'Release',
  takeOver: 'Take over',
  destroy: 'Destroy',
}

export const SEVERITY = {
  error: { rank: 0, label: 'Problem', accent: 'bg-problem', text: 'text-problem', icon: '!' },
  warning: { rank: 1, label: 'Warning', accent: 'bg-caution', text: 'text-caution', icon: '!' },
  info: { rank: 2, label: 'Notice', accent: 'bg-yours', text: 'text-yours', icon: 'i' },
}

// What resolves each kind of finding: the action this panel takes for it, and
// — where there is one — the command it amounts to for whoever would rather
// type it. Only commands that exist: one that does not is worse than none,
// since the plan behind each action already shows exactly what runs. A finding
// without a fix is a notification, and notifications get ignored — and a fix
// that does nothing when pressed is worse, so every remedy names an action
// the dashboard really performs.
//
//   act: start | open | edit-route | remove-route | add-route | take-over | issue-tls
export function remediesFor(finding, { instances, routes, runtime = 'lxc' }) {
  const bin = runtime === 'incus' ? 'incus' : 'lxc'
  const subject = finding.subject
  const route = routes.find((r) => r.domain === subject)
  const served = route && instances.find((i) => i.address === route.target)
  const owner = served ?? instances.find((i) => i.name === subject || i.domain === subject)

  switch (finding.kind) {
    case 'target-down':
      return served
        ? [{ act: 'start', target: served, label: `Start ${served.name}`, command: `${bin} start ${served.name}` }]
        : []
    case 'nothing-listening':
      return served
        ? [{ act: 'open', target: served, label: `Open ${served.name}`, command: `${bin} exec ${served.name} -- systemctl --failed` }]
        : []
    case 'stale-route':
      return route
        ? [{ act: 'edit-route', target: route, label: 'Point it at the container' }]
        : []
    case 'orphan-route':
      return route
        ? [
            { act: 'edit-route', target: route, label: 'Point it at a container' },
            { act: 'remove-route', target: route, label: 'Stop serving it' },
          ]
        : []
    case 'missing-route':
      return owner
        ? [{ act: 'add-route', target: owner, label: `Serve ${owner.domain}` }]
        : []
    case 'unmanaged':
      return route
        ? [{ act: 'take-over', target: route, label: VERBS.takeOver }]
        : []
    case 'certificate-expired':
    case 'certificate-expiring':
    case 'certificate-self-signed':
      if (!route) return []
      if (route.state === 'unmanaged') {
        return [{ act: 'take-over', target: route, label: 'Take it over to renew it' }]
      }
      if (route.state === 'managed' && !route.certificates?.startsWith('/var/lib/croft/')) {
        return [{ act: 'issue-tls', target: route, label: 'Renew with croft' }]
      }
      return []
    default:
      return []
  }
}

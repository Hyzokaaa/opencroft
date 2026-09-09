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
    label: 'external',
    tone: 'border-edge-strong text-muted',
    explain: 'Created outside OpenCroft. Listed, never modified.',
  },
}

export const SEVERITY = {
  error: { rank: 0, label: 'Problem', accent: 'bg-problem', text: 'text-problem', icon: '!' },
  warning: { rank: 1, label: 'Warning', accent: 'bg-caution', text: 'text-caution', icon: '!' },
  info: { rank: 2, label: 'Notice', accent: 'bg-yours', text: 'text-yours', icon: 'i' },
}

// The command that resolves each kind of finding. A finding without a fix is
// a notification, and notifications get ignored.
export function remedyFor(finding, instances) {
  const subject = finding.subject
  const owner = instances.find((i) => i.domain === subject || i.name === subject)

  switch (finding.kind) {
    case 'target-down':
      return { label: `Start ${owner?.name ?? subject}`, command: `croft start ${owner?.name ?? subject}` }
    case 'stale-route':
      return { label: 'Repoint the route', command: `croft route sync ${subject}` }
    case 'orphan-route':
      return { label: 'Remove the route', command: `croft route rm ${subject}` }
    case 'missing-route':
      return { label: 'Create the route', command: `croft route add ${owner?.domain ?? subject} --target ${owner?.name ?? ''}`.trim() }
    case 'unmanaged':
      return { label: 'Adopt', command: `croft adopt ${subject}` }
    default:
      return null
  }
}

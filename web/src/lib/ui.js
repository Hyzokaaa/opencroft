// The buttons every page shares, so the one that matters looks the same
// wherever it is. PRIMARY is used once per screen. Both are 36px tall: a
// target the size of a line of small text was hard to hit and harder to see
// as the thing to press.
export const PRIMARY =
  'inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-md bg-action px-3.5 text-sm font-medium text-ground transition hover:bg-action/90 disabled:opacity-50'
export const SECONDARY =
  'inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-md border border-edge-strong px-3 text-sm text-ink transition hover:bg-raised disabled:opacity-50'
// GHOST is an action that is there without asking to be pressed: a page's
// secondary verbs beside its one primary.
export const GHOST =
  'inline-flex h-9 items-center whitespace-nowrap rounded-md px-2.5 text-sm text-muted transition hover:bg-raised hover:text-ink'
export const DANGER =
  'inline-flex h-9 items-center whitespace-nowrap rounded-md border border-problem/50 px-3 text-sm text-problem transition hover:bg-problem/10'
export const QUIET = 'text-xs text-muted underline-offset-4 transition hover:text-ink hover:underline'

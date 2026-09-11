package entities

import (
	"sort"
	"strings"
	"time"
)

// Snapshots come from two places and must never be confused.
//
// Yours are the ones you took, for your reasons, at moments only you know
// about. Ours are taken automatically, immediately before a deployment, so
// that there is a point to come back to.
//
// The rule that separates them is the one already used for generated vhosts:
// the prefix says who made it. Anything starting with "croft-" is ours and we
// may remove it as it ages. Anything else is yours and is never touched — not
// when pruning, not by accident, not ever.
//
//	croft-deploy-backend-20260911-143000   ours, prunable
//	before-upgrade                          yours, untouchable
//
// The shape is croft-<kind>-<subject>-<timestamp>. Adding another kind later
// — an upgrade, a scheduled backup — fits without reopening the decision.
const (
	Prefix     = "croft-"
	DeployKind = Prefix + "deploy-"

	// stamp sorts lexically as it sorts in time, and is UTC so that a host
	// that changes timezone does not reorder its own history.
	stamp = "20060102-150405"
)

// SnapshotName says what was deployed and when. The snapshot covers the whole
// container — a service cannot be snapshotted on its own — so the name records
// which deployment caused it, not what it contains.
func SnapshotName(service string, at time.Time) string {
	return DeployKind + service + "-" + at.UTC().Format(stamp)
}

// Ours is the whole of the ownership rule. Everything that removes a snapshot
// asks this first.
func Ours(snapshot string) bool { return strings.HasPrefix(snapshot, Prefix) }

// OfService is true for our snapshots taken for this particular service, so
// that pruning one deployment's history never reaches into another's.
func OfService(snapshot, service string) bool {
	return strings.HasPrefix(snapshot, DeployKind+service+"-")
}

// Prunable returns the snapshots to remove: ours, for this service, beyond the
// most recent keep — and never the one recorded as healthy, which is the whole
// point of recording it.
//
// The result is what a plan step will show. Removing snapshots silently would
// be the one place this tool deleted something nobody read about.
func Prunable(snapshots []string, service string, keep int, healthy string) []string {
	mine := []string{}
	for _, snapshot := range snapshots {
		if OfService(snapshot, service) && snapshot != healthy {
			mine = append(mine, snapshot)
		}
	}

	// Names sort by time, so the tail is the oldest.
	sort.Sort(sort.Reverse(sort.StringSlice(mine)))

	if keep < 0 {
		keep = 0
	}
	if len(mine) <= keep {
		return nil
	}
	return mine[keep:]
}

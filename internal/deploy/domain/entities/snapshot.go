package entities

import (
	"sort"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/snapshot"
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
// The shape is croft-<kind>-<subject>-<timestamp>. The prefix, the timestamp
// and the ownership rule live in shared/snapshot, because the database module
// names snapshots too and there can only be one answer to "is this ours".
const (
	Prefix     = snapshot.Prefix
	DeployKind = Prefix + "deploy-"

	// DestroyKind is taken before a service is removed. Pruning only ever
	// reaches DeployKind, so this one survives — it is the only record of
	// something that was deliberately deleted, and the only way back.
	DestroyKind = Prefix + "destroy-"
)

// SnapshotName says what was deployed and when. The snapshot covers the whole
// container — a service cannot be snapshotted on its own — so the name records
// which deployment caused it, not what it contains.
func SnapshotName(service string, at time.Time) string {
	return snapshot.Name(DeployKind, service, at)
}

// Ours is the whole of the ownership rule. Everything that removes a snapshot
// asks this first.
func Ours(name string) bool { return snapshot.Ours(name) }

// OfService is true for our snapshots taken for this particular service, so
// that pruning one deployment's history never reaches into another's.
func OfService(name, service string) bool {
	return strings.HasPrefix(name, DeployKind+service+"-")
}

// Prunable returns the snapshots to remove: ours, for this service, beyond the
// most recent keep — and never the one recorded as healthy, which is the whole
// point of recording it.
//
// The result is what a plan step will show. Removing snapshots silently would
// be the one place this tool deleted something nobody read about.
func Prunable(snapshots []string, service string, keep int, healthy string) []string {
	mine := []string{}
	for _, name := range snapshots {
		if OfService(name, service) && name != healthy {
			mine = append(mine, name)
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

// FarewellName marks the moment a service was removed. It is deliberately not
// a DeployKind: pruning must never reach it, because it is the only way back
// to something somebody chose to delete.
func FarewellName(service string, at time.Time) string {
	return snapshot.Name(DestroyKind, service, at)
}

// IndexKey lists the services on a container. Without it there is no way to
// enumerate what is deployed — only to ask about names already known.
const IndexKey = "user.croft.services"

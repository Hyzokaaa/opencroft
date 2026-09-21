// Package snapshot holds the one rule that decides whether croft may remove a
// snapshot: the prefix says who made it.
//
// It lives here rather than inside a module because a third consumer arrived.
// The deploy module names the snapshots taken around a deployment and the
// database module names the ones taken around provisioning; if each carried
// its own copy of the prefix, "is this ours" would stop being a single rule
// and become two that agree until they do not.
package snapshot

import (
	"strings"
	"time"
)

const (
	// Prefix marks a snapshot as ours. Anything without it belongs to whoever
	// took it and is never touched — not when pruning, not by accident.
	Prefix = "croft-"

	// Stamp sorts lexically as it sorts in time, and is UTC so that a host
	// that changes timezone does not reorder its own history.
	Stamp = "20060102-150405"
)

// Name is croft-<kind>-<subject>-<timestamp>. Kind carries its own trailing
// dash, so that the constants read as the prefixes they are.
func Name(kind, subject string, at time.Time) string {
	return kind + subject + "-" + at.UTC().Format(Stamp)
}

// Ours is the whole of the ownership rule. Everything that removes a snapshot
// asks this first.
func Ours(name string) bool { return strings.HasPrefix(name, Prefix) }

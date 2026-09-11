package entities_test

import (
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
)

// The prefix is the whole ownership rule, and pruning is the one place this
// tool removes something on its own. A snapshot somebody took by hand, for
// reasons only they know, must survive everything we do.
func TestSnapshotsWeDidNotTakeAreNeverOurs(t *testing.T) {
	theirs := []string{"before-upgrade", "monday", "backup-2026", "deploy-backend-20260911-140000"}

	for _, snapshot := range theirs {
		if entities.Ours(snapshot) {
			t.Errorf("%q was claimed as ours", snapshot)
		}
	}

	if !entities.Ours(entities.SnapshotName("backend", time.Now())) {
		t.Error("our own snapshot was not recognised")
	}
}

func TestPruningNeverReachesSomebodyElsesSnapshots(t *testing.T) {
	all := []string{
		"before-upgrade",
		"monday",
		entities.SnapshotName("backend", at(1)),
		entities.SnapshotName("backend", at(2)),
		entities.SnapshotName("backend", at(3)),
		entities.SnapshotName("backend", at(4)),
	}

	for _, snapshot := range entities.Prunable(all, "backend", 2, "") {
		if !entities.Ours(snapshot) {
			t.Errorf("pruning would remove %q, which we did not take", snapshot)
		}
	}
}

// One service's history is not another's. A container running three services
// keeps three sets.
func TestPruningOneServiceLeavesTheOthersAlone(t *testing.T) {
	all := []string{
		entities.SnapshotName("backend", at(1)),
		entities.SnapshotName("backend", at(2)),
		entities.SnapshotName("backend", at(3)),
		entities.SnapshotName("client", at(1)),
		entities.SnapshotName("client", at(2)),
	}

	for _, snapshot := range entities.Prunable(all, "backend", 1, "") {
		if entities.OfService(snapshot, "client") {
			t.Errorf("pruning backend would remove %q", snapshot)
		}
	}
}

// The oldest go first, and exactly `keep` survive.
func TestPruningKeepsTheMostRecent(t *testing.T) {
	oldest, middle, newest := entities.SnapshotName("backend", at(1)),
		entities.SnapshotName("backend", at(2)),
		entities.SnapshotName("backend", at(3))

	prunable := entities.Prunable([]string{middle, newest, oldest}, "backend", 2, "")

	if len(prunable) != 1 || prunable[0] != oldest {
		t.Fatalf("got %v, wanted just %s", prunable, oldest)
	}
}

// Recording which snapshot was healthy is the whole reason "go back to the
// last version that worked" means anything. Pruning it away would quietly
// break that promise.
func TestTheHealthySnapshotIsNeverPruned(t *testing.T) {
	healthy := entities.SnapshotName("backend", at(1))
	all := []string{
		healthy,
		entities.SnapshotName("backend", at(2)),
		entities.SnapshotName("backend", at(3)),
		entities.SnapshotName("backend", at(4)),
	}

	for _, snapshot := range entities.Prunable(all, "backend", 1, healthy) {
		if snapshot == healthy {
			t.Fatal("the healthy snapshot was pruned")
		}
	}
}

func TestNothingIsPrunedWhileThereIsRoom(t *testing.T) {
	all := []string{entities.SnapshotName("backend", at(1))}

	if prunable := entities.Prunable(all, "backend", 3, ""); len(prunable) != 0 {
		t.Errorf("got %v", prunable)
	}
}

// Names have to sort by time, because that is how the oldest is found.
func TestSnapshotNamesSortByWhenTheyWereTaken(t *testing.T) {
	first, second := entities.SnapshotName("backend", at(1)), entities.SnapshotName("backend", at(2))

	if !(first < second) {
		t.Errorf("%s does not sort before %s", first, second)
	}
	if entities.SnapshotName("backend", at(1)) == entities.SnapshotName("client", at(1)) {
		t.Error("two services share a snapshot name")
	}
}

func at(hour int) time.Time {
	return time.Date(2026, 9, 11, hour, 0, 0, 0, time.UTC)
}

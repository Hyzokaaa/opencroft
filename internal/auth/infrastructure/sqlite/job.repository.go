package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/job"
)

// JobJournal keeps what croft did: the one record the system itself cannot
// give back. A container shows what it runs now, not that the deployment
// before this one failed at its build step.
//
// Each job is one row. The columns are what the history is sorted and settled
// by; the body is the job as it was written down, secrets already left out.
type JobJournal struct {
	store *Store
}

// Kept is how many jobs the journal holds. Months of deployments on a busy
// host, and a table that stays small enough to read whole.
const Kept = 1000

func NewJobJournal(store *Store) *JobJournal {
	return &JobJournal{store: store}
}

// Settle marks whatever is still recorded as running as interrupted. It is
// called when the daemon starts, before it runs anything, so nothing recorded
// as running can be its own: those are the jobs a restart walked away from.
func (j *JobJournal) Settle(ctx context.Context) error {
	_, err := j.store.db.ExecContext(ctx,
		`UPDATE jobs SET status = ? WHERE status = ?`, job.StatusInterrupted, job.StatusRunning)
	return err
}

func (j *JobJournal) Record(snapshot job.Snapshot) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if _, err := j.store.db.ExecContext(ctx, `
		INSERT INTO jobs (id, kind, subject, status, started, body) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET status = excluded.status, body = excluded.body`,
		snapshot.Id, snapshot.Kind, snapshot.Subject, snapshot.Status,
		snapshot.Started.UTC().Format(time.RFC3339Nano), string(body)); err != nil {
		return err
	}

	_, err = j.store.db.ExecContext(ctx, `
		DELETE FROM jobs WHERE id NOT IN (SELECT id FROM jobs ORDER BY started DESC LIMIT ?)`, Kept)
	return err
}

func (j *JobJournal) Recent(limit int) ([]job.Snapshot, error) {
	rows, err := j.store.db.QueryContext(context.Background(),
		`SELECT status, body FROM jobs ORDER BY started DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := []job.Snapshot{}
	for rows.Next() {
		var status, body string
		if err := rows.Scan(&status, &body); err != nil {
			return nil, err
		}
		snapshot, err := decode(status, body)
		if err != nil {
			return nil, err
		}
		found = append(found, snapshot)
	}
	return found, rows.Err()
}

func (j *JobJournal) Find(id string) (job.Snapshot, bool, error) {
	var status, body string
	err := j.store.db.QueryRowContext(context.Background(),
		`SELECT status, body FROM jobs WHERE id = ?`, id).Scan(&status, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return job.Snapshot{}, false, nil
	}
	if err != nil {
		return job.Snapshot{}, false, err
	}
	snapshot, err := decode(status, body)
	return snapshot, err == nil, err
}

// decode takes the status from its column: settling rewrites the column, not
// the body the job was written down with.
func decode(status, body string) (job.Snapshot, error) {
	var snapshot job.Snapshot
	if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
		return job.Snapshot{}, err
	}
	snapshot.Status = job.Status(status)
	return snapshot, nil
}

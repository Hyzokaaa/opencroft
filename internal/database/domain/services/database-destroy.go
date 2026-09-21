package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
)

var ErrNotProvisioned = errors.New("this container has no database by that name")

// Removal is everything the plan needs that is not the database itself: the
// real keys on the container, and the index it should be left with.
type Removal struct {
	Keys      []string
	Remaining []string
}

// DestroyDatabase decides what removing a database reaches. It builds from
// what the container actually holds, so the plan names the real keys rather
// than the ones they would be by default.
type DestroyDatabase struct{}

func NewDestroyDatabase() *DestroyDatabase { return &DestroyDatabase{} }

func (s *DestroyDatabase) Execute(
	config map[string]string, name string,
) (*entities.Database, Removal, error) {
	if err := ValidName(name); err != nil {
		return nil, Removal{}, err
	}

	index := Stored(config)
	if !contains(index, name) {
		return nil, Removal{}, ErrNotProvisioned
	}

	found := FromConfig(config, name)
	if found == nil {
		return nil, Removal{}, ErrNotProvisioned
	}

	return found, Removal{
		Keys:      Keys(config, name),
		Remaining: Without(index, name),
	}, nil
}

// Consequences is what stops being true once the data is gone.
//
// This is the one operation in the module that destroys something a snapshot
// is the only way back from, so it says so plainly, and it names the services
// that are about to lose their connection rather than leaving that to be
// discovered when they restart.
func Consequences(database *entities.Database, services []string) string {
	said := []string{
		fmt.Sprintf("%s and everything in it is dropped", database.DB),
	}

	if len(services) > 0 {
		said = append(said, fmt.Sprintf(
			"%s %s its credentials and will fail to connect",
			strings.Join(services, ", "), verb(len(services))))
	}

	said = append(said,
		"a snapshot is taken first, and it is the only way back")

	return strings.Join(said, ". ") + "."
}

func verb(n int) string {
	if n == 1 {
		return "loses"
	}
	return "lose"
}

// Warning is what a person has to read before restoring a snapshot once a
// database lives in the container.
//
// It is the same capability sold as the advantage and warned about as the
// risk: the restore brings the data back to that moment, which is the whole
// point, and takes everything written since with it, which is the cost. Not
// saying the second half is how the first half stops being believed.
func Warning(databases []*entities.Database, snapshot string) string {
	local := []string{}
	away := []string{}

	for _, database := range databases {
		if database.Local() {
			local = append(local, database.DB)
			continue
		}
		away = append(away, database.DB+" in "+database.Location)
	}

	said := []string{}
	if len(local) > 0 {
		said = append(said, fmt.Sprintf(
			"%s goes back to %s too, losing anything written since",
			strings.Join(local, ", "), snapshot))
	}
	if len(away) > 0 {
		said = append(said, fmt.Sprintf(
			"%s is somewhere else and does NOT go back — the application will "+
				"return to an older version against today's data",
			strings.Join(away, ", ")))
	}

	if len(said) == 0 {
		return ""
	}
	return strings.Join(said, ". ") + "."
}

func contains(all []string, want string) bool {
	for _, item := range all {
		if item == want {
			return true
		}
	}
	return false
}

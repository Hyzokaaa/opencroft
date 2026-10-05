package services

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
)

var (
	ErrNotFoundHere   = errors.New("no database by that name is running in this container")
	ErrNameUnworkable = errors.New("croft can only take on a database whose name is lowercase letters, digits and underscores — rename it, or keep using it as it is")
)

// Found is a database running in a container that croft did not create: the
// engine, the database, who owns it and the port the engine listens on, as
// the engine itself reports them.
type Found struct {
	Engine enums.Engine
	DB     string
	Owner  string
	Port   int
}

// ParseFound reads what the agent's look inside a container printed: one
// database per line, engine|database|owner|port. Anything else is skipped
// rather than guessed at.
func ParseFound(output string) []Found {
	found := []Found{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 4 {
			continue
		}
		engine, ok := enums.ParseEngine(strings.TrimSpace(fields[0]))
		db := strings.TrimSpace(fields[1])
		if !ok || db == "" {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(fields[3]))
		if err != nil || port <= 0 {
			port = engine.Port()
		}
		found = append(found, Found{Engine: engine, DB: db, Owner: strings.TrimSpace(fields[2]), Port: port})
	}
	return found
}

// Unrecorded is what was found that croft has not taken note of yet.
func Unrecorded(config map[string]string, found []Found) []Found {
	known := map[string]bool{}
	for _, d := range All(config) {
		if d.Local() {
			known[string(d.Engine)+"/"+d.DB] = true
		}
	}
	out := []Found{}
	for _, f := range found {
		if !known[string(f.Engine)+"/"+f.DB] {
			out = append(out, f)
		}
	}
	return out
}

// AdoptDatabase decides what taking note of a found database records: the
// engine, the database and its owner, the port — and that croft did not make
// it, so letting it go never drops it.
//
// Its name has to be a plain identifier, because it reaches SQL unquoted when
// another container of the project connects to it. A database called
// something else can still be used exactly as before; croft only declines to
// manage it.
func AdoptDatabase(config map[string]string, found Found) (*entities.Database, []string, error) {
	if ValidName(found.DB) != nil {
		return nil, nil, ErrNameUnworkable
	}
	owner := found.Owner
	if found.Engine == enums.EnginePostgres {
		if ValidName(owner) != nil {
			return nil, nil, ErrNameUnworkable
		}
	} else {
		owner = found.DB
	}

	index := Stored(config)
	for _, existing := range index {
		if existing == found.DB {
			return nil, nil, ErrAlreadyExists
		}
	}

	return entities.NewDatabase(entities.DatabaseProps{
		Name: found.DB, Engine: found.Engine, DB: found.DB, User: owner,
		Port: found.Port, Adopted: true,
	}), With(index, found.DB), nil
}

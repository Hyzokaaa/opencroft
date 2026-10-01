package services

import (
	"errors"
	"regexp"
	"sort"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
)

var (
	ErrSameContainer  = errors.New("that database is already in this container")
	ErrOtherProject   = errors.New("only a container in the same project connects to a database — put both in one project first")
	ErrNotShareable   = errors.New("redis has no password, so croft does not open it to other containers — keep it beside what uses it")
	ErrNotThere       = errors.New("that container has no database by that name")
	ErrSecondHand     = errors.New("that database lives in yet another container — connect to the one that holds it")
	ErrStillConnected = errors.New("other containers are connected to this database")
)

// Side is one end of a connection, as the agent knows it.
type Side struct {
	Name    string
	Project string
	Config  map[string]string
}

// ShareDatabase decides whether the consumer may connect to a database in the
// provider, and what that leaves it with: the database as the provider holds
// it, the consumer's own login, and its index afterwards.
//
// Only inside one project. A project is the containers that belong together;
// a database opened to anything outside it is a dependency nobody grouped,
// and the first rollback of the provider would find it.
func ShareDatabase(consumer, provider Side, name string) (*entities.Database, string, []string, error) {
	switch {
	case consumer.Name == provider.Name:
		return nil, "", nil, ErrSameContainer
	case consumer.Project == "" || consumer.Project != provider.Project:
		return nil, "", nil, ErrOtherProject
	}

	provided := FromConfig(provider.Config, name)
	switch {
	case provided == nil:
		return nil, "", nil, ErrNotThere
	case !provided.Local():
		return nil, "", nil, ErrSecondHand
	case !provided.Engine.Credentialed():
		return nil, "", nil, ErrNotShareable
	}

	index := Stored(consumer.Config)
	for _, existing := range index {
		if existing == name {
			return nil, "", nil, ErrAlreadyExists
		}
		if found := FromConfig(consumer.Config, existing); found != nil && found.Engine == provided.Engine {
			return nil, "", nil, ErrEngineTaken
		}
	}

	return provided, Login(consumer.Name), With(index, name), nil
}

var notIdentifier = regexp.MustCompile(`[^a-z0-9_]`)

// Login is the consumer's own user in the provider's engine, named after the
// container so that a listing of logins says who each one is for.
func Login(consumer string) string {
	login := "from_" + notIdentifier.ReplaceAllString(lower(consumer), "_")
	if len(login) > 31 {
		login = login[:31]
	}
	return login
}

func lower(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 'a' - 'A'
		}
	}
	return string(out)
}

// Consumers are the containers connected to one database in the provider.
// Like Dependents, it is computed from the consumers' own records, so there is
// no second list to drift.
func Consumers(configs map[string]map[string]string, provider, name string) []string {
	found := []string{}
	for container, config := range configs {
		for _, database := range All(config) {
			if database.Location == provider && database.Name == name {
				found = append(found, container)
				break
			}
		}
	}
	sort.Strings(found)
	return found
}

// Shareable is every database a container could connect to: the ones its
// project's other containers hold, with a password, that it has no engine of
// its own to clash with.
func Shareable(consumer Side, others []Side) []Offer {
	offers := []Offer{}
	for _, provider := range others {
		for _, database := range All(provider.Config) {
			if _, _, _, err := ShareDatabase(consumer, provider, database.Name); err == nil {
				offers = append(offers, Offer{Container: provider.Name, Database: database})
			}
		}
	}
	sort.Slice(offers, func(a, b int) bool {
		if offers[a].Container != offers[b].Container {
			return offers[a].Container < offers[b].Container
		}
		return offers[a].Database.Name < offers[b].Database.Name
	})
	return offers
}

type Offer struct {
	Container string
	Database  *entities.Database
}

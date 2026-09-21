// Package services holds the business rules of a provisioned database.
//
// Everything here is a pure function over data the caller already has: the
// configuration of a container, as one map. That is deliberate — it means the
// rules can be tested without LXD, without root and without a container, which
// is the same bargain the reconciliation engine makes.
package services

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
)

// Key is one database's annotation, as the runtime spells it.
func Key(name, key string) string { return entities.KeyPrefix + name + "." + key }

// Stored reads the index. Without it there is no way to enumerate what is on a
// container, only to ask about names already known.
func Stored(config map[string]string) []string {
	return strings.Fields(config[entities.IndexKey])
}

// FromConfig rebuilds one database out of a configuration already in hand.
// Returns nil when the container knows nothing about that name.
func FromConfig(config map[string]string, name string) *entities.Database {
	read := func(key string) string { return config[Key(name, key)] }

	engine, ok := enums.ParseEngine(read("engine"))
	if !ok {
		return nil
	}

	port, _ := strconv.Atoi(read("port"))

	return entities.NewDatabase(entities.DatabaseProps{
		Name:     name,
		Engine:   engine,
		DB:       read("db"),
		User:     read("user"),
		Port:     port,
		Location: read("location"),
	})
}

// All reads every database on a container in one pass over its configuration.
// A name in the index that carries no readable engine is skipped rather than
// guessed at: half a database is not a database.
func All(config map[string]string) []*entities.Database {
	names := Stored(config)
	out := make([]*entities.Database, 0, len(names))

	for _, name := range names {
		if found := FromConfig(config, name); found != nil {
			out = append(out, found)
		}
	}
	return out
}

// Keys returns every annotation that belongs to one database, so that removing
// it names the real keys rather than the ones it would have by default.
func Keys(config map[string]string, name string) []string {
	prefix := Key(name, "")
	keys := []string{}

	for key := range config {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// Without returns the index with one name taken out, so that what is written
// back never disagrees with what is actually on the container.
func Without(index []string, name string) []string {
	remaining := []string{}
	for _, other := range index {
		if other != name {
			remaining = append(remaining, other)
		}
	}
	return remaining
}

// With returns the index with one name added, sorted, and unchanged when it is
// already there.
func With(index []string, name string) []string {
	for _, existing := range index {
		if existing == name {
			return index
		}
	}

	out := append(append([]string{}, index...), name)
	sort.Strings(out)
	return out
}

package services_test

import (
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/services"
)

// config turns a database into the annotations a container would carry, which
// is how everything here is read back.
func config(databases ...*entities.Database) map[string]string {
	out := map[string]string{}
	names := []string{}

	for _, database := range databases {
		names = append(names, database.Name)
		for _, a := range database.Annotations() {
			if a[1] != "" {
				out[services.Key(database.Name, a[0])] = a[1]
			}
		}
	}
	out[entities.IndexKey] = strings.Join(names, " ")
	return out
}

func postgres(name string) *entities.Database {
	return entities.NewDatabase(entities.DatabaseProps{
		Name: name, Engine: enums.EnginePostgres,
	})
}

// A database name reaches SQL as an identifier. Lowercase, digits and
// underscores can never need quoting, so there is no quoting to get wrong.
func TestADatabaseNameHasToBeAnSQLIdentifier(t *testing.T) {
	for _, name := range []string{
		"main-db",   // a dash cannot appear in an unquoted identifier
		"Main",      // folding case is how two names become one
		"1main",     // an identifier does not start with a digit
		"main;drop", // the shape of an injection
		"main db",   // a space
		"",          // nothing at all
		strings.Repeat("a", 32),
	} {
		if err := services.ValidName(name); err == nil {
			t.Errorf("%q was accepted as a database name", name)
		}
	}

	for _, name := range []string{"main", "app_data", "cache2"} {
		if err := services.ValidName(name); err != nil {
			t.Errorf("%q was refused: %v", name, err)
		}
	}
}

func TestAnUnknownEngineIsRefused(t *testing.T) {
	_, _, err := services.NewProvisionDatabase().Execute(
		services.ProvisionProps{Name: "main", Engine: "oracle"}, map[string]string{})

	if err != services.ErrEngineUnknown {
		t.Fatalf("an unknown engine gave %v", err)
	}
}

// Two engines of the same kind in one container would both claim DATABASE_URL.
// Refusing says what the collision is; inventing a prefix scheme would not.
func TestTwoOfTheSameEngineAreRefused(t *testing.T) {
	existing := config(postgres("main"))

	_, _, err := services.NewProvisionDatabase().Execute(
		services.ProvisionProps{Name: "other", Engine: "postgres"}, existing)

	if err != services.ErrEngineTaken {
		t.Fatalf("a second postgres gave %v", err)
	}

	// A different engine beside it is fine: they claim different variables.
	if _, _, err := services.NewProvisionDatabase().Execute(
		services.ProvisionProps{Name: "cache", Engine: "redis"}, existing); err != nil {
		t.Fatalf("redis beside postgres was refused: %v", err)
	}
}

func TestANameAlreadyThereIsRefused(t *testing.T) {
	_, _, err := services.NewProvisionDatabase().Execute(
		services.ProvisionProps{Name: "main", Engine: "redis"}, config(postgres("main")))

	if err != services.ErrAlreadyExists {
		t.Fatalf("a repeated name gave %v", err)
	}
}

// What is written onto the container has to be what comes back off it, or the
// panel is describing something that is not there.
func TestADatabaseSurvivesTheRoundTrip(t *testing.T) {
	original := entities.NewDatabase(entities.DatabaseProps{
		Name: "main", Engine: enums.EngineMySQL, DB: "appdb", User: "app",
	})

	read := services.FromConfig(config(original), "main")
	if read == nil {
		t.Fatal("the database did not come back")
	}

	if read.Engine != original.Engine || read.DB != original.DB ||
		read.User != original.User || read.Port != original.Port {
		t.Fatalf("came back as %+v, went in as %+v", read, original)
	}
	if !read.Local() {
		t.Error("a database with no location came back as living elsewhere")
	}
}

// A name in the index with nothing readable behind it is skipped, not guessed
// at. Half a database is not a database.
func TestAHalfWrittenDatabaseIsNotListed(t *testing.T) {
	found := services.All(map[string]string{entities.IndexKey: "main"})

	if len(found) != 0 {
		t.Fatalf("listed %d databases from an index alone", len(found))
	}
}

func TestRemovingOneLeavesTheIndexSayingWhatRemains(t *testing.T) {
	both := config(postgres("main"), entities.NewDatabase(
		entities.DatabaseProps{Name: "cache", Engine: enums.EngineRedis}))

	_, removal, err := services.NewDestroyDatabase().Execute(both, "main")
	if err != nil {
		t.Fatal(err)
	}

	if len(removal.Remaining) != 1 || removal.Remaining[0] != "cache" {
		t.Fatalf("the index would be left as %v", removal.Remaining)
	}
	for _, key := range removal.Keys {
		if !strings.Contains(key, ".main.") {
			t.Errorf("removing main would forget %q", key)
		}
	}
}

// The dependency is recorded only on the side that consumes it, so who depends
// on what is computed rather than stored — there is no second copy to drift.
func TestDependentsAreFoundFromTheConsumerSide(t *testing.T) {
	away := entities.NewDatabase(entities.DatabaseProps{
		Name: "main", Engine: enums.EnginePostgres, Location: "data",
	})

	configs := map[string]map[string]string{
		"data": {},
		"app":  config(away),
		"idle": {},
	}

	found := services.Dependents(configs, "data")
	if len(found) != 1 || found[0] != "app" {
		t.Fatalf("dependents of data came back as %v", found)
	}

	if services.Refusal("data", found) == "" {
		t.Error("destroying a container others read from says nothing")
	}
}

// The same capability sold as the advantage and warned about as the risk. Not
// saying the second half is how the first half stops being believed.
func TestTheRollbackWarningSaysWhatHappensToTheData(t *testing.T) {
	inside := services.Warning(
		[]*entities.Database{postgres("main")}, "croft-deploy-backend-20260911-143000")

	if !strings.Contains(inside, "losing anything written since") {
		t.Errorf("a local database does not warn about losing data: %q", inside)
	}

	outside := services.Warning([]*entities.Database{
		entities.NewDatabase(entities.DatabaseProps{
			Name: "main", Engine: enums.EnginePostgres, Location: "data",
		}),
	}, "croft-deploy-backend-20260911-143000")

	if !strings.Contains(outside, "does NOT go back") {
		t.Errorf("a database elsewhere does not warn that it stays put: %q", outside)
	}
}

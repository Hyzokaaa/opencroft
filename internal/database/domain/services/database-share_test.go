package services

import (
	"errors"
	"testing"
)

func holding(name, engine string) map[string]string {
	return map[string]string{
		"user.croft.databases":                    name,
		"user.croft.database." + name + ".engine": engine,
		"user.croft.database." + name + ".db":     name,
		"user.croft.database." + name + ".user":   name,
	}
}

// A database is shared inside one project, from the container that holds
// it, and only with a password to give.
func TestADatabaseIsSharedWithinItsProjectOnly(t *testing.T) {
	backend := Side{Name: "openhelpdesk", Project: "helpdesk", Config: holding("helpdesk", "postgres")}
	web := Side{Name: "cloud-openhelpdesk", Project: "helpdesk", Config: map[string]string{}}

	provided, login, index, err := ShareDatabase(web, backend, "helpdesk")
	if err != nil {
		t.Fatal(err)
	}
	if provided.User != "helpdesk" || login != "from_cloud_openhelpdesk" || len(index) != 1 || index[0] != "helpdesk" {
		t.Errorf("got %+v %s %v", provided, login, index)
	}

	elsewhere := Side{Name: "billing", Project: "billing", Config: map[string]string{}}
	loose := Side{Name: "loose", Config: map[string]string{}}
	cache := Side{Name: "cache", Project: "helpdesk", Config: holding("cache", "redis")}
	relayed := Side{Name: "relay", Project: "helpdesk", Config: map[string]string{
		"user.croft.databases": "helpdesk", "user.croft.database.helpdesk.engine": "postgres",
		"user.croft.database.helpdesk.location": "openhelpdesk",
	}}
	clash := Side{Name: "api", Project: "helpdesk", Config: holding("own", "postgres")}

	for _, c := range []struct {
		consumer, provider Side
		name               string
		want               error
	}{
		{elsewhere, backend, "helpdesk", ErrOtherProject},
		{loose, Side{Name: "x", Config: holding("helpdesk", "postgres")}, "helpdesk", ErrOtherProject},
		{backend, backend, "helpdesk", ErrSameContainer},
		{web, cache, "cache", ErrNotShareable},
		{web, relayed, "helpdesk", ErrSecondHand},
		{web, backend, "nothing", ErrNotThere},
		{clash, backend, "helpdesk", ErrEngineTaken},
	} {
		if _, _, _, err := ShareDatabase(c.consumer, c.provider, c.name); !errors.Is(err, c.want) {
			t.Errorf("%s → %s/%s: got %v, want %v", c.consumer.Name, c.provider.Name, c.name, err, c.want)
		}
	}
}

// A login is an SQL identifier made from the container's name.
func TestALoginIsNamedAfterItsContainer(t *testing.T) {
	for container, want := range map[string]string{
		"cloud-openhelpdesk": "from_cloud_openhelpdesk",
		"Web2":               "from_web2",
		"a-very-long-container-name-for-a-service": "from_a_very_long_container_name",
	} {
		got := Login(container)
		if got != want || ValidName(got) != nil {
			t.Errorf("%s: got %q", container, got)
		}
	}
}

// Who reads a database is read from the readers, and only for that database.
func TestTheConsumersOfADatabaseAreFoundOnTheirSide(t *testing.T) {
	configs := map[string]map[string]string{
		"web": {
			"user.croft.databases": "helpdesk", "user.croft.database.helpdesk.engine": "postgres",
			"user.croft.database.helpdesk.location": "openhelpdesk",
		},
		"other": holding("helpdesk", "postgres"),
	}
	if got := Consumers(configs, "openhelpdesk", "helpdesk"); len(got) != 1 || got[0] != "web" {
		t.Errorf("got %v", got)
	}
	if got := Consumers(configs, "openhelpdesk", "billing"); len(got) != 0 {
		t.Errorf("another database's readers: %v", got)
	}
}

// Package entities holds what a provisioned database is.
//
// A Database is not a thing beside an Instance — it is something a container
// holds, like a Service. That is the whole point of the module: because the
// data lives inside the container, `lxc snapshot` captures an application and
// its database at the same instant. Two containers would be two snapshots that
// are not consistent with each other, which is the problem every PaaS built on
// images has and the reason this one is worth building.
//
// Everything about a database lives as annotations on the container — except
// the password, which never leaves it.
package entities

import (
	"strconv"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/snapshot"
)

const (
	// ProvisionKind is taken before a database is installed. Pruning only ever
	// reaches the deploy kind, so this survives on purpose.
	ProvisionKind = snapshot.Prefix + "provision-"

	// FarewellKind is taken before a database is dropped. It carries its own
	// kind rather than reusing the service one, so that a database and a
	// service sharing a name cannot produce the same snapshot twice.
	FarewellKind = snapshot.Prefix + "db-destroy-"

	// IndexKey lists the databases on a container. Without it there is no way
	// to enumerate them, only to ask about names already known.
	IndexKey = "user.croft.databases"

	// KeyPrefix namespaces one database's own keys. It is spelled out here
	// rather than borrowed from the runtime package, because the domain does
	// not get to depend on infrastructure to know its own annotation names.
	KeyPrefix = "user.croft.database."

	// Dir is where the generated credentials live inside the container: one
	// file per database, readable by root only.
	Dir = "/etc/croft/db.d"

	// EnvPath is the single file every unit reads. It is regenerated from Dir
	// whenever a database is provisioned or dropped, so that adding one never
	// means rewriting a unit.
	EnvPath = "/etc/croft/db.env"
)

// Database is one engine running inside a container, with one database and one
// user created for the application beside it.
type Database struct {
	// Name identifies it within the container and names its credentials file.
	Name string

	Engine enums.Engine

	// DB and User are what was created inside the engine. They are SQL
	// identifiers, which is why they are validated harder than a service name.
	DB   string
	User string

	Port int

	// Location is the container the engine runs in. Empty means this one,
	// which is the default and the only shape that keeps a snapshot atomic.
	// A name here means the data is somewhere else, and that rolling this
	// container back will not roll the data back with it.
	Location string

	// Adopted is a database croft found running and took note of, rather than
	// created. Its credentials are whoever made it's, kept wherever they were;
	// letting it go forgets the note and never touches the data.
	Adopted bool
}

type DatabaseProps struct {
	Name     string
	Engine   enums.Engine
	DB       string
	User     string
	Port     int
	Location string
	Adopted  bool
}

func NewDatabase(props DatabaseProps) *Database {
	port := props.Port
	if port == 0 {
		port = props.Engine.Port()
	}

	db := props.DB
	if db == "" {
		db = props.Name
	}

	user := props.User
	if user == "" {
		user = props.Name
	}

	return &Database{
		Name:     props.Name,
		Engine:   props.Engine,
		DB:       db,
		User:     user,
		Port:     port,
		Location: props.Location,
		Adopted:  props.Adopted,
	}
}

// Local is true when the data lives in the same container as the application.
// It is the case the product exists for, and the only one where restoring a
// snapshot restores the data too.
func (d *Database) Local() bool { return d.Location == "" }

// Host is what the application connects to. Inside one container that is the
// loopback, which is also why nothing has to be opened up to reach it.
func (d *Database) Host(address string) string {
	if d.Local() {
		return "127.0.0.1"
	}
	return address
}

// EnvFile is this database's own credentials, written once at provisioning and
// never rewritten by a deployment. The file a person edits is a different one.
func (d *Database) EnvFile() string { return Dir + "/" + d.Name + ".env" }

// ProvisionName is the snapshot taken before the engine is installed.
func (d *Database) ProvisionName(at time.Time) string {
	return snapshot.Name(ProvisionKind, d.Name, at)
}

// FarewellName is the snapshot taken before the data is dropped. It is the
// only way back from a deletion somebody chose.
func (d *Database) FarewellName(at time.Time) string {
	return snapshot.Name(FarewellKind, d.Name, at)
}

// Annotations is the metadata recorded on the container. The password is
// deliberately absent: these are readable by the unprivileged half, which
// serves them to a browser.
func (d *Database) Annotations() [][2]string {
	return [][2]string{
		{"engine", string(d.Engine)},
		{"db", d.DB},
		{"user", d.User},
		{"port", strconv.Itoa(d.Port)},
		{"location", d.Location},
		{"adopted", adopted(d.Adopted)},
	}
}

func adopted(yes bool) string {
	if yes {
		return "true"
	}
	return ""
}

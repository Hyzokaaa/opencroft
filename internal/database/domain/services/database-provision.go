package services

import (
	"errors"
	"regexp"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
)

var (
	ErrNameInvalid   = errors.New("a database name is lowercase letters, digits and underscores, starting with a letter")
	ErrEngineUnknown = errors.New("croft knows the engines postgres, mysql and redis")
	ErrAlreadyExists = errors.New("this container already has a database by that name")
	ErrEngineTaken   = errors.New("this container already runs that engine, and two of them would both claim DATABASE_URL")
	ErrLocationAway  = errors.New("a database somewhere else needs a container name croft would recognise")
)

// namePattern is tighter than a service name and deliberately so: this reaches
// SQL as an identifier. Lowercase, digits and underscores can never need
// quoting, which means there is no quoting to get wrong.
//
// No dashes, because an unquoted SQL identifier cannot carry one — and quoting
// it would be one more thing that has to stay correct forever.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// ValidName is exported because the agent checks it again on its own side. It
// cannot assume the panel is the one calling, and one definition of what a
// valid identifier is beats two that agree until they drift.
func ValidName(name string) error {
	if !namePattern.MatchString(name) {
		return ErrNameInvalid
	}
	return nil
}

type ProvisionProps struct {
	Name   string
	Engine string
	DB     string
	User   string

	// Location is the container the engine runs in. Empty is the same
	// container as the application, which is the only shape where restoring a
	// snapshot restores the data with it.
	Location string
}

// ProvisionDatabase decides what a database will be. It does not run anything:
// what comes back is an entity and the index the container should be left
// with, and the planner turns that into commands somebody reads first.
type ProvisionDatabase struct{}

func NewProvisionDatabase() *ProvisionDatabase { return &ProvisionDatabase{} }

// Execute validates the request against what the container already holds.
func (s *ProvisionDatabase) Execute(
	props ProvisionProps, config map[string]string,
) (*entities.Database, []string, error) {
	if err := ValidName(props.Name); err != nil {
		return nil, nil, err
	}

	engine, ok := enums.ParseEngine(props.Engine)
	if !ok {
		return nil, nil, ErrEngineUnknown
	}

	db, user := props.DB, props.User
	if db == "" {
		db = props.Name
	}
	if user == "" {
		user = props.Name
	}
	if engine.Credentialed() {
		if err := ValidName(db); err != nil {
			return nil, nil, err
		}
		if err := ValidName(user); err != nil {
			return nil, nil, err
		}
	}

	if props.Location != "" && !containerPattern.MatchString(props.Location) {
		return nil, nil, ErrLocationAway
	}

	index := Stored(config)
	for _, existing := range index {
		if existing == props.Name {
			return nil, nil, ErrAlreadyExists
		}
		// Two engines of the same kind in one container would both want to be
		// DATABASE_URL. Refusing is better than inventing a prefix scheme
		// nobody asked for, and the message says what the collision is.
		if found := FromConfig(config, existing); found != nil && found.Engine == engine {
			return nil, nil, ErrEngineTaken
		}
	}

	return entities.NewDatabase(entities.DatabaseProps{
		Name: props.Name, Engine: engine, DB: db, User: user,
		Location: props.Location,
	}), With(index, props.Name), nil
}

// containerPattern is the runtime's own shape for a container name, which is
// looser than an SQL identifier because it never reaches SQL.
var containerPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)

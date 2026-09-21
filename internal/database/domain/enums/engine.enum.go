package enums

// Engine is a closed set on purpose.
//
// It reaches the agent from a request and ends up choosing a package name and
// a command. A free string here would be the panel telling the privileged half
// what to install; an enum is the panel choosing from what the agent was
// already willing to install.
type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
	EngineRedis    Engine = "redis"
)

// ParseEngine refuses anything it does not already know.
func ParseEngine(value string) (Engine, bool) {
	switch Engine(value) {
	case EnginePostgres:
		return EnginePostgres, true
	case EngineMySQL:
		return EngineMySQL, true
	case EngineRedis:
		return EngineRedis, true
	default:
		return "", false
	}
}

func (e Engine) Valid() bool {
	_, ok := ParseEngine(string(e))
	return ok
}

// Port is where the engine listens by default. It is a property of the
// protocol, not of the distribution, which is why it lives here and the
// package name does not.
func (e Engine) Port() int {
	switch e {
	case EnginePostgres:
		return 5432
	case EngineMySQL:
		return 3306
	case EngineRedis:
		return 6379
	default:
		return 0
	}
}

// Credentialed is false for an engine we provision without a user of its own.
// Redis reached over the loopback inside one container is guarded by the
// container, not by a password nobody will ever rotate.
func (e Engine) Credentialed() bool { return e != EngineRedis }

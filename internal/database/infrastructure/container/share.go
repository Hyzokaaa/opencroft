package container

import (
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A database is shared by opening it to one more container, not by moving it.
//
// The data stays where it was, beside the application that owns it, so a
// snapshot of that container still holds both. The container that connects
// gets a login of its own — so letting it go later is dropping one login, not
// changing a password everybody shares — and is let in from its own address
// only. The password is made inside the container that holds the data and
// travels to the other through a pipe on this host; it is never in a plan,
// never on the socket, never in the panel.

// Share is one container connecting to a database in another.
type Share struct {
	Bin string

	// Provider holds the data; Consumer connects to it.
	Provider, ProviderAddress string
	Consumer, ConsumerAddress string

	// Host is what the consumer connects to: the provider's name on the
	// bridge when it has one, which survives the address changing.
	Host string

	// Database is the one in the provider, as recorded there. Its User owns
	// the data; Login is the consumer's own.
	Database *entities.Database
	Login    string
}

// Connected is the database as the consumer records it: the same engine, name
// and port, its own login, and where it really lives.
func (s Share) Connected() *entities.Database {
	return entities.NewDatabase(entities.DatabaseProps{
		Name: s.Database.Name, Engine: s.Database.Engine, DB: s.Database.DB,
		User: s.Login, Port: s.Database.Port, Location: s.Provider,
	})
}

// handOff is where the provider leaves the credentials for the one moment
// between making them and the consumer taking them.
func (s Share) handOff() string {
	return entities.Dir + "/granted/" + s.Consumer + "-" + s.Database.Name + ".env"
}

func (s Share) inProvider(describe, command string) plan.Step {
	return plan.Command(describe, s.Bin, "exec", s.Provider, "--", "sh", "-lc", command)
}

func (s Share) inConsumer(describe, command string) plan.Step {
	return plan.Command(describe, s.Bin, "exec", s.Consumer, "--", "sh", "-lc", command)
}

// Plan opens the database to the consumer and hands it the credentials.
func (s Share) Plan(index, services []string) plan.Plan {
	d := s.Database
	steps := []plan.Step{
		s.inProvider("Let "+string(d.Engine)+" in "+s.Provider+" accept connections from "+
			s.Consumer+" ("+s.ConsumerAddress+") only — a restart of the engine if it listened on the loopback alone",
			s.open()),
		s.inProvider("Wait until "+string(d.Engine)+" accepts connections", waitFor(d.Engine)),
		s.inProvider("Create the login "+s.Login+" for "+s.Consumer+", with a password generated inside "+s.Provider,
			s.grant()),
		plan.Command("Hand the credentials to "+s.Consumer+" through a pipe on this host, and keep no copy in "+s.Provider,
			"sh", "-c", s.carry()),
	}

	for _, a := range s.Connected().Annotations() {
		if a[1] == "" {
			continue
		}
		steps = append(steps, plan.Command("Record "+a[0]+" on "+s.Consumer,
			s.Bin, "config", "set", s.Consumer, entities.KeyPrefix+d.Name+"."+a[0], a[1]))
	}
	steps = append(steps, plan.Command("List "+strings.Join(index, ", ")+" in the index of "+s.Consumer,
		s.Bin, "config", "set", s.Consumer, entities.IndexKey, strings.Join(index, " ")))

	planner := NewPlanner(s.Bin, s.Consumer)
	for _, service := range services {
		steps = append(steps, plan.Optional("Restart croft-"+service+" so it reads the new credentials",
			s.Bin, "exec", s.Consumer, "--", "sh", "-lc", planner.reread(service)))
	}
	return plan.New(steps...)
}

// hba is the one line that lets the consumer in, and is what letting it go
// takes back out.
func (s Share) hba() string {
	return "host " + s.Database.DB + " " + s.Login + " " + s.ConsumerAddress + "/32 scram-sha-256"
}

// open makes the engine listen where the consumer can reach it and lets that
// one address in. Idempotent: a second consumer adds its line and nothing else.
func (s Share) open() string {
	switch s.Database.Engine {
	case enums.EnginePostgres:
		return "set -e\n" +
			"current=$(su -s /bin/sh postgres -c \"psql -tAc 'SHOW listen_addresses'\")\n" +
			"restart=\n" +
			"case \",$current,\" in\n" +
			"  *,'*',*|*," + s.ProviderAddress + ",*) ;;\n" +
			"  *) su -s /bin/sh postgres -c 'psql -v ON_ERROR_STOP=1' <<SQL\n" +
			"ALTER SYSTEM SET listen_addresses TO '$current," + s.ProviderAddress + "';\n" +
			"SQL\n" +
			"     restart=yes ;;\n" +
			"esac\n" +
			"hba=$(su -s /bin/sh postgres -c \"psql -tAc 'SHOW hba_file'\")\n" +
			"grep -qxF '" + s.hba() + "' \"$hba\" || echo '" + s.hba() + "' >> \"$hba\"\n" +
			"if [ -n \"$restart\" ]; then systemctl restart postgresql\n" +
			"else su -s /bin/sh postgres -c \"psql -c 'SELECT pg_reload_conf()'\" >/dev/null; fi"
	default:
		// MariaDB lets a user in by the address it connects from, so the
		// engine can listen on the bridge and the login still only works from
		// the consumer.
		conf := "/etc/mysql/mariadb.conf.d/99-croft.cnf"
		return "set -e\n" +
			"if [ ! -f " + conf + " ]; then\n" +
			"  printf '[mysqld]\\nbind-address = 0.0.0.0\\n' > " + conf + "\n" +
			"  systemctl restart mariadb\n" +
			"fi"
	}
}

// grant creates the consumer's login and leaves its credentials, once, where
// the next step picks them up. The password exists in this shell and that
// file, and nowhere else.
func (s Share) grant() string {
	d := s.Database
	port := strconv.Itoa(d.Port)
	head := "set -e\ninstall -d -m 0700 " + entities.Dir + "/granted\n" + generate + "\n"

	switch d.Engine {
	case enums.EnginePostgres:
		// A member of the owner's role, so it reads and writes what the owner
		// can without the owner's password changing hands.
		return head +
			"su -s /bin/sh postgres -c 'psql -v ON_ERROR_STOP=1' <<SQL\n" +
			"CREATE USER " + s.Login + " PASSWORD '$PW' IN ROLE " + d.User + ";\n" +
			"SQL\n" +
			s.leave([]string{
				"DATABASE_URL=postgres://" + s.Login + ":$PW@" + s.Host + ":" + port + "/" + d.DB,
				"PGHOST=" + s.Host,
				"PGPORT=" + port,
				"PGUSER=" + s.Login,
				"PGPASSWORD=$PW",
				"PGDATABASE=" + d.DB,
			})
	default:
		return head +
			"mariadb -e \"CREATE USER '" + s.Login + "'@'" + s.ConsumerAddress + "' IDENTIFIED BY '$PW'; " +
			"GRANT ALL PRIVILEGES ON " + d.DB + ".* TO '" + s.Login + "'@'" + s.ConsumerAddress + "'; " +
			"FLUSH PRIVILEGES;\"\n" +
			s.leave([]string{
				"DATABASE_URL=mysql://" + s.Login + ":$PW@" + s.Host + ":" + port + "/" + d.DB,
				"MYSQL_HOST=" + s.Host,
				"MYSQL_PORT=" + port,
				"MYSQL_USER=" + s.Login,
				"MYSQL_PASSWORD=$PW",
				"MYSQL_DATABASE=" + d.DB,
			})
	}
}

func (s Share) leave(lines []string) string {
	return "umask 077\ncat > " + s.handOff() + " <<ENV\n" + strings.Join(lines, "\n") + "\nENV"
}

// carry moves the credentials from one container to the other. The consumer
// refuses an empty file, which is what a failed read on the other side of the
// pipe would hand it.
func (s Share) carry() string {
	target := s.Connected().EnvFile()
	return "set -e\n" +
		s.Bin + " exec " + s.Provider + " -- cat " + s.handOff() + " | " +
		s.Bin + " exec " + s.Consumer + " -- sh -c 'set -e; install -d -m 0700 " + entities.Dir +
		"; umask 077; cat > " + target + ".new; [ -s " + target + ".new ]; mv " + target + ".new " + target +
		"; " + strings.ReplaceAll(regenerate, "\n", "; ") + "'\n" +
		s.Bin + " exec " + s.Provider + " -- rm -f " + s.handOff()
}

// ── Letting go ────────────────────────────────────────────────────────────────

// Revoke takes the consumer's login and its line back out of the provider,
// and the credentials out of the consumer. The data is not touched: it was
// never the consumer's.
//
// The provider's half is optional. A provider already gone has nothing left
// to revoke, and that must not keep the consumer from forgetting it.
func (s Share) Revoke(keys, remaining, services []string) plan.Plan {
	d := s.Database
	steps := []plan.Step{}

	switch d.Engine {
	case enums.EnginePostgres:
		steps = append(steps, plan.Optional("Drop the login "+s.Login+" in "+s.Provider+
			" — what it created passes to "+d.User+" — and stop letting "+s.ConsumerAddress+" in",
			s.Bin, "exec", s.Provider, "--", "sh", "-lc",
			"set -e\n"+
				"su -s /bin/sh postgres -c 'psql -v ON_ERROR_STOP=1 -d "+d.DB+"' <<SQL\n"+
				"REASSIGN OWNED BY "+s.Login+" TO "+d.User+";\n"+
				"DROP OWNED BY "+s.Login+";\n"+
				"DROP USER IF EXISTS "+s.Login+";\n"+
				"SQL\n"+
				"hba=$(su -s /bin/sh postgres -c \"psql -tAc 'SHOW hba_file'\")\n"+
				"sed -i '\\|^"+s.hba()+"$|d' \"$hba\"\n"+
				"su -s /bin/sh postgres -c \"psql -c 'SELECT pg_reload_conf()'\" >/dev/null"))
	default:
		steps = append(steps, plan.Optional("Drop the login "+s.Login+" in "+s.Provider,
			s.Bin, "exec", s.Provider, "--", "sh", "-lc",
			"mariadb -e \"DROP USER IF EXISTS '"+s.Login+"'@'"+s.ConsumerAddress+"';\""))
	}

	connected := s.Connected()
	steps = append(steps, s.inConsumer("Remove "+connected.EnvFile()+" from "+s.Consumer+" and rebuild "+entities.EnvPath,
		"set -e\nrm -f "+connected.EnvFile()+"\n"+regenerate))

	for _, service := range services {
		steps = append(steps, plan.Optional("Restart croft-"+service+", which no longer has these credentials",
			s.Bin, "exec", s.Consumer, "--", "systemctl", "restart", "croft-"+service))
	}
	for _, key := range keys {
		steps = append(steps, plan.Optional("Forget "+key, s.Bin, "config", "unset", s.Consumer, key))
	}
	if len(remaining) > 0 {
		steps = append(steps, plan.Command("Leave "+strings.Join(remaining, ", ")+" in the index",
			s.Bin, "config", "set", s.Consumer, entities.IndexKey, strings.Join(remaining, " ")))
	} else {
		steps = append(steps, plan.Optional("Empty the index", s.Bin, "config", "unset", s.Consumer, entities.IndexKey))
	}
	return plan.New(steps...)
}

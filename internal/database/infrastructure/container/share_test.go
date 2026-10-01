package container

import (
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
)

func aShare(engine enums.Engine) Share {
	return Share{
		Bin:      "lxc",
		Provider: "openhelpdesk", ProviderAddress: "10.210.68.50",
		Consumer: "cloud-openhelpdesk", ConsumerAddress: "10.210.68.51",
		Host: "openhelpdesk.lxd",
		Database: entities.NewDatabase(entities.DatabaseProps{
			Name: "helpdesk", Engine: engine, DB: "helpdesk", User: "helpdesk",
		}),
		Login: "from_cloud_openhelpdesk",
	}
}

// Connecting lets one address in, makes a login of its own, and carries the
// password across on this host — never writing it into the plan.
func TestConnectingOpensTheDatabaseToOneAddress(t *testing.T) {
	shell := aShare(enums.EnginePostgres).Plan([]string{"helpdesk"}, []string{"web"}).Shell()

	for _, want := range []string{
		"host helpdesk from_cloud_openhelpdesk 10.210.68.51/32 scram-sha-256",
		"listen_addresses",
		"CREATE USER from_cloud_openhelpdesk PASSWORD '$PW' IN ROLE helpdesk",
		"postgres://from_cloud_openhelpdesk:$PW@openhelpdesk.lxd:5432/helpdesk",
		"lxc exec openhelpdesk -- cat /etc/croft/db.d/granted/cloud-openhelpdesk-helpdesk.env | lxc exec cloud-openhelpdesk",
		"rm -f /etc/croft/db.d/granted/cloud-openhelpdesk-helpdesk.env",
		"user.croft.database.helpdesk.location openhelpdesk",
		"user.croft.database.helpdesk.user from_cloud_openhelpdesk",
		"systemctl restart croft-web",
	} {
		if !strings.Contains(shell, want) {
			t.Errorf("the plan lacks %q:\n%s", want, shell)
		}
	}
	// The password is made by the step that uses it; the plan only ever
	// names the variable.
	if strings.Contains(shell, "PW=") && !strings.Contains(shell, generate) {
		t.Errorf("a password is set some other way:\n%s", shell)
	}
}

// MariaDB lets a user in by where it connects from.
func TestAMariaDBLoginIsBoundToTheConsumersAddress(t *testing.T) {
	shell := aShare(enums.EngineMySQL).Plan([]string{"helpdesk"}, nil).Shell()
	if !strings.Contains(shell, "CREATE USER 'from_cloud_openhelpdesk'@'10.210.68.51'") ||
		!strings.Contains(shell, "GRANT ALL PRIVILEGES ON helpdesk.* TO 'from_cloud_openhelpdesk'@'10.210.68.51'") {
		t.Errorf("plan:\n%s", shell)
	}
}

// Letting go drops the login and its line, never the data, and still lets the
// consumer forget a provider that is already gone.
func TestDisconnectingDropsTheLoginNotTheData(t *testing.T) {
	p := aShare(enums.EnginePostgres).Revoke([]string{"user.croft.database.helpdesk.engine"}, nil, []string{"web"})
	shell := p.Shell()

	if !strings.Contains(shell, "DROP USER IF EXISTS from_cloud_openhelpdesk") ||
		!strings.Contains(shell, "REASSIGN OWNED BY from_cloud_openhelpdesk TO helpdesk") {
		t.Errorf("the login is not dropped:\n%s", shell)
	}
	if strings.Contains(shell, "DROP DATABASE") || strings.Contains(shell, "snapshot") {
		t.Errorf("disconnecting reaches the data:\n%s", shell)
	}
	if !p.Steps[0].Optional {
		t.Error("a provider that is gone would keep the consumer from letting go")
	}
}

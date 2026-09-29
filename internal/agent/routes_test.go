package agent

import (
	"net/http"
	"testing"
)

// Every field of a route reaches a file nginx loads as root.
func TestARouteCannotCarryAnythingElse(t *testing.T) {
	server, _ := testServer()
	base := RouteDTO{Domain: "app.example.com", Target: "10.0.0.5", Port: 80}

	for name, broken := range map[string]func(*RouteDTO){
		"a prefix with a space":      func(d *RouteDTO) { d.Paths = []PathDTO{{Prefix: "/a b/", Target: "10.0.0.5", Port: 1}} },
		"a prefix ending a block":    func(d *RouteDTO) { d.Paths = []PathDTO{{Prefix: "/api/;}", Target: "10.0.0.5", Port: 1}} },
		"a prefix without its slash": func(d *RouteDTO) { d.Paths = []PathDTO{{Prefix: "/api", Target: "10.0.0.5", Port: 1}} },
		"a prefix climbing out":      func(d *RouteDTO) { d.Paths = []PathDTO{{Prefix: "/api/../", Target: "10.0.0.5", Port: 1}} },
		"a path to a name":           func(d *RouteDTO) { d.Paths = []PathDTO{{Prefix: "/api/", Target: "evil.com", Port: 1}} },
		"a certificate anywhere":     func(d *RouteDTO) { d.Certificates = "/root/.ssh" },
		"a certificate climbing out": func(d *RouteDTO) { d.Certificates = "/etc/letsencrypt/live/../../shadow" },
		"a certificate one level up": func(d *RouteDTO) { d.Certificates = "/etc/letsencrypt/live/.." },
	} {
		dto := base
		broken(&dto)
		if code := serve(server, http.MethodPost, "/routes/plan", dto).Code; code == http.StatusOK {
			t.Errorf("%s was accepted", name)
		}
	}

	dto := base
	dto.Paths = []PathDTO{{Prefix: "/api/", Target: "10.0.0.5", Port: 3000, Strip: true}}
	dto.Certificates = "/etc/letsencrypt/live/app.example.com"
	if code := serve(server, http.MethodPost, "/routes/plan", dto).Code; code != http.StatusOK {
		t.Errorf("a reasonable route was refused with %d", code)
	}
}

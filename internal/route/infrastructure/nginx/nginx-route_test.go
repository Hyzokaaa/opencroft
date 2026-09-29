package nginx

import (
	"context"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// The panel narrates plans as they run, over a connection that stays open and
// is often quiet. nginx buffers proxied responses by default, which holds every
// event back until the work has finished — the panel then shows 0/N from
// beginning to end — and its default read timeout cuts a quiet connection after
// a minute.
//
// This only appeared once the panel was given a domain of its own. Over an ssh
// tunnel it reaches the port directly and there is no proxy to buffer anything.
func TestTheGeneratedVhostLetsStreamsThrough(t *testing.T) {
	for _, ssl := range []bool{false, true} {
		route := entities.NewRoute(entities.RouteProps{
			Domain: "app.example.com", Target: "10.0.0.200", Port: 3000, SSL: ssl,
		})

		body := render(route)
		for _, needed := range []string{"proxy_buffering off", "proxy_read_timeout"} {
			if !strings.Contains(body, needed) {
				t.Errorf("ssl=%v: %s is missing from the vhost:\n%s", ssl, needed, body)
			}
		}
	}
}

// A websocket starts as HTTP and asks to switch. Without these three lines in
// every location nginx drops the asking, and the backend refuses the upgrade.
func TestEveryLocationLetsAWebsocketUpgrade(t *testing.T) {
	route := entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80, SSL: true,
		Paths: []entities.PathRoute{{Prefix: "/api/", Target: "10.0.0.200", Port: 3000, Strip: true}},
	})
	body := render(route)

	for _, needed := range []string{
		"proxy_http_version 1.1;", "proxy_set_header Upgrade $http_upgrade;", "proxy_set_header Connection $http_connection;",
	} {
		if count := strings.Count(body, needed); count != 2 {
			t.Errorf("%q appears in %d of 2 locations:\n%s", needed, count, body)
		}
	}
}

// A stripped path reaches its backend without the prefix; one that is not
// stripped keeps it. The difference is one slash in proxy_pass.
func TestAPathIsPassedOnWithOrWithoutItsPrefix(t *testing.T) {
	route := entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80,
		Paths: []entities.PathRoute{
			{Prefix: "/api/", Target: "10.0.0.201", Port: 3000, Strip: true},
			{Prefix: "/files/", Target: "10.0.0.202", Port: 9000},
		},
	})
	body := render(route)

	for _, wanted := range []string{
		"location / {\n        proxy_pass http://10.0.0.200:80;",
		"location /api/ {\n        proxy_pass http://10.0.0.201:3000/;",
		"location /files/ {\n        proxy_pass http://10.0.0.202:9000;",
	} {
		if !strings.Contains(body, wanted) {
			t.Errorf("missing %q in:\n%s", wanted, body)
		}
	}
}

// What croft writes it reads back as the same route — paths, strip and
// certificate included — or an edit would quietly drop them.
func TestARouteReadsBackAsItWasWritten(t *testing.T) {
	written := entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80, SSL: true,
		Certificates: "/etc/letsencrypt/live/app.example.com",
		Paths:        []entities.PathRoute{{Prefix: "/api/", Target: "10.0.0.200", Port: 3000, Strip: true}},
	})

	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")
	file := "/etc/nginx/croft.d/app.example.com.conf"
	fake.Files[file] = marked(written)
	fake.Responses["nginx -T"] = "# configuration file " + file + ":\n" + marked(written)

	read, err := repository.FindByDomain(context.Background(), "app.example.com")
	if err != nil || read == nil {
		t.Fatalf("read %v, %v", read, err)
	}
	if read.Target != "10.0.0.200" || read.Port != 80 || !read.SSL || read.Certificates != written.Certificates {
		t.Errorf("read back as %+v", read)
	}
	if len(read.Paths) != 1 || read.Paths[0] != written.Paths[0] {
		t.Errorf("paths read back as %+v", read.Paths)
	}
	if read.State != "managed" {
		t.Errorf("croft's own file reads as %s", read.State)
	}
}

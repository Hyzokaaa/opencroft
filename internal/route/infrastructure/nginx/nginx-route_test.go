package nginx

import (
	"context"
	"errors"
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

// A return written beside the locations runs before nginx looks at any of
// them, so Let's Encrypt would be redirected away from its token. On https the
// redirect has to be a location of its own.
func TestTheChallengeIsNotRedirectedAway(t *testing.T) {
	body := render(entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80, SSL: true,
	}))

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "    return") {
			t.Fatalf("a return at server level:\n%s", body)
		}
	}
	if !strings.Contains(body, "location / {\n        return 301 https://$host$request_uri;") {
		t.Errorf("port 80 no longer sends visitors to https:\n%s", body)
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

// A route with aliases is one file and reads back as one route: each https
// name with its own certificate, a name still waiting for one over http, and
// every name answering with the same paths.
func TestAliasesAreOneRouteWithACertificateEach(t *testing.T) {
	written := entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80, SSL: true,
		Certificates: "/var/lib/croft/certificates/_.example.com",
		Paths:        []entities.PathRoute{{Prefix: "/api/", Target: "10.0.0.200", Port: 3000, Strip: true}},
		Aliases: []entities.Alias{
			{Domain: "help.customer.com", SSL: true},
			{Domain: "support.other.org"},
		},
	})
	body := render(written)
	if strings.Count(body, "listen 443 ssl;") != 2 || strings.Count(body, "location /api/ {") != 3 ||
		!strings.Contains(body, "ssl_certificate /var/lib/croft/certificates/help.customer.com/fullchain.pem;") {
		t.Errorf("rendered:\n%s", body)
	}

	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")
	file := "/etc/nginx/croft.d/app.example.com.conf"
	fake.Files[file] = marked(written)
	fake.Responses["nginx -T"] = "# configuration file " + file + ":\n" + marked(written)

	all, err := repository.FindAll(context.Background())
	if err != nil || len(all) != 1 {
		t.Fatalf("read %d routes: %v", len(all), err)
	}
	read := all[0]
	if read.Domain != "app.example.com" || len(read.Aliases) != 2 {
		t.Fatalf("read back as %+v", read)
	}
	if a := read.Aliases[0]; a.Domain != "help.customer.com" || !a.SSL || a.CertDir() != "/var/lib/croft/certificates/help.customer.com" {
		t.Errorf("the https alias: %+v", a)
	}
	if a := read.Aliases[1]; a.Domain != "support.other.org" || a.SSL {
		t.Errorf("the alias without a certificate: %+v", a)
	}
}

// A rewrite nginx rejects puts the previous file back — it used to remove the
// file, and with it the domain.
func TestARejectedRewriteKeepsTheDomain(t *testing.T) {
	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")
	file := "/etc/nginx/croft.d/app.example.com.conf"
	fake.Files[file] = "the file that was there"
	fake.Failures["nginx -t"] = errors.New("invalid")

	_ = repository.Write(context.Background(), entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.200", Port: 80,
	}))
	if fake.Files[file] != "the file that was there" {
		t.Errorf("after a rejected rewrite the file is %q", fake.Files[file])
	}
}

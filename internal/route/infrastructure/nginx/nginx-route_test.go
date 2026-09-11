package nginx

import (
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
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

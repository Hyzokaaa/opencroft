// Package nginx implements RouteRepository over nginx configuration files.
//
// Generated files carry a hash of their own body. If the file on disk no longer
// matches, a human edited it — we show the drift and stop managing it rather
// than overwriting their work.
package nginx

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

const (
	Marker  = "# managed-by: croft"
	hashKey = "# croft-hash: "
)

// sites-available/sites-enabled is a Debian convention. Owning a directory and
// adding a single include keeps this working on any distribution.
type NginxRouteRepository struct {
	host    host.Host
	confDir string
}

func NewNginxRouteRepository(h host.Host, confDir string) *NginxRouteRepository {
	return &NginxRouteRepository{host: h, confDir: confDir}
}

// FindAll reads the configuration nginx actually loaded, not our own
// directory. A panel that only sees what it wrote itself cannot honestly claim
// to list what it did not create.
func (r *NginxRouteRepository) FindAll(ctx context.Context) ([]*entities.Route, error) {
	out, err := r.host.Run(ctx, "nginx", "-T")
	if err != nil {
		return nil, fmt.Errorf("reading the nginx configuration: %w", err)
	}

	// Several blocks share one domain: typically an http block that redirects
	// and an https block that proxies. They are one route to a person.
	merged := map[string]*entities.Route{}
	order := []string{}

	for _, block := range scanServerBlocks(out.Stdout) {
		target, port := upstream(block.Body)

		for _, domain := range serverNames(block.Body) {
			existing, seen := merged[domain]
			if !seen {
				existing = entities.NewRoute(entities.RouteProps{
					Domain: domain,
					File:   block.File,
					State:  r.stateOfFile(ctx, block.File),
				})
				merged[domain] = existing
				order = append(order, domain)
			}

			if target != "" && existing.Target == "" {
				existing.Target = target
				existing.Port = port
			}
			if servesTLS(block.Body) {
				existing.SSL = true
			}
		}
	}

	routes := make([]*entities.Route, 0, len(order))
	for _, domain := range order {
		routes = append(routes, merged[domain])
	}
	return routes, nil
}

// stateOfFile decides how much authority we have over the file a block came
// from. Anything outside our own directory is external by definition.
func (r *NginxRouteRepository) stateOfFile(ctx context.Context, path string) enums.ManagedState {
	if !strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(r.confDir)+"/") {
		return enums.StateUnmanaged
	}

	content, err := r.host.ReadFile(ctx, path)
	if err != nil {
		return enums.StateUnmanaged
	}
	return stateOf(string(content))
}

func (r *NginxRouteRepository) FindByDomain(ctx context.Context, domain string) (*entities.Route, error) {
	all, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, route := range all {
		if route.Domain == domain {
			return route, nil
		}
	}
	return nil, nil
}

// stateOf compares the recorded hash with the body actually on disk.
func stateOf(content string) enums.ManagedState {
	if !strings.HasPrefix(content, Marker) {
		return enums.StateUnmanaged
	}

	lines := strings.SplitN(content, "\n", 4)
	if len(lines) < 4 {
		return enums.StateAdopted
	}

	recorded := strings.TrimPrefix(lines[1], hashKey)
	if recorded == bodyHash(lines[3]) {
		return enums.StateManaged
	}
	return enums.StateAdopted
}

func bodyHash(body string) string {
	sum := md5.Sum([]byte(body))
	return hex.EncodeToString(sum[:])
}

func (r *NginxRouteRepository) Write(ctx context.Context, route *entities.Route) error {
	return r.walk(ctx, r.WritePlan(route))
}

// walk undoes the files it wrote if a later step fails.
//
// Leaving a rejected vhost on disk is worse than it sounds: nginx keeps
// running on its old configuration, so nothing looks broken — until the next
// reload, by anybody, for any reason, fails and takes every site with it. The
// blast radius belongs to whoever wrote the bad file, not to the next person
// who restarts nginx.
func (r *NginxRouteRepository) walk(ctx context.Context, p plan.Plan) error {
	written := []string{}

	for _, step := range p.Steps {
		err := host.RunStep(ctx, r.host, step)
		if err == nil {
			if step.IsFile() {
				written = append(written, step.File)
			}
			continue
		}

		if step.Optional {
			continue
		}

		for _, path := range written {
			_ = r.host.RemoveFile(ctx, path)
		}
		if len(written) > 0 {
			return fmt.Errorf("%w (the file was removed again, so nginx is unchanged)", err)
		}
		return err
	}
	return nil
}

func (r *NginxRouteRepository) Remove(ctx context.Context, domain string) error {
	return r.walk(ctx, r.RemovePlan(domain))
}

func (r *NginxRouteRepository) Reload(ctx context.Context) error {
	if _, err := r.host.Run(ctx, "nginx", "-t"); err != nil {
		return fmt.Errorf("nginx rejected the configuration: %w", err)
	}
	if _, ok := r.host.Lookup(ctx, "systemctl"); ok {
		_, err := r.host.Run(ctx, "systemctl", "reload", "nginx")
		return err
	}
	_, err := r.host.Run(ctx, "rc-service", "nginx", "reload")
	return err
}

// acmeChallenge lets Let's Encrypt reach the token over port 80 without
// anything having to take the port away from nginx.
const acmeChallenge = `    location /.well-known/acme-challenge/ {
        root /var/lib/croft/acme;
    }
`

func render(route *entities.Route) string {
	if !route.SSL {
		return fmt.Sprintf(`server {
    listen 80;
    server_name %s;

%s
    location / {
        proxy_pass http://%s:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, route.Domain, acmeChallenge, route.Target, route.Port)
	}

	return fmt.Sprintf(`server {
    listen 80;
    server_name %s;

%s
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name %s;

    ssl_certificate %s/fullchain.pem;
    ssl_certificate_key %s/privkey.pem;

    location / {
        proxy_pass http://%s:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, route.Domain, acmeChallenge, route.Domain, route.CertDir(), route.CertDir(), route.Target, route.Port)
}

// WritePlan is what adding a domain does, in the order it happens. Validating
// before reloading matters: a bad file that nginx accepts is a bug, a bad file
// that nginx rejects would take every other site down with it.
func (r *NginxRouteRepository) WritePlan(route *entities.Route) plan.Plan {
	path := filepath.Join(r.confDir, route.Domain+".conf")

	return plan.New(
		plan.WriteFile("Write the vhost, with a hash of its own contents", path, marked(route)),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", "systemctl", "reload", "nginx"),
	)
}

func (r *NginxRouteRepository) RemovePlan(domain string) plan.Plan {
	path := filepath.Join(r.confDir, domain+".conf")

	return plan.New(
		plan.Command("Remove the vhost", "rm", "-f", path),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", "systemctl", "reload", "nginx"),
	)
}

// marked renders the file with the header that lets us tell later whether
// somebody edited it by hand.
func marked(route *entities.Route) string {
	body := render(route)
	return fmt.Sprintf("%s\n%s%s\n# Edited by hand? This file will no longer be managed automatically.\n%s",
		Marker, hashKey, bodyHash(body), body)
}

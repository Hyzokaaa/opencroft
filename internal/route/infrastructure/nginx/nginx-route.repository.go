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
	"sort"
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
		paths := []entities.PathRoute{}
		for _, loc := range proxiedLocations(block.Body) {
			if loc.Prefix == "/" {
				target, port = loc.Host, loc.Port
				continue
			}
			paths = append(paths, entities.PathRoute{
				Prefix: loc.Prefix, Target: loc.Host, Port: loc.Port, Strip: loc.Strip,
			})
		}
		certificates := certificateDir(block.Body)

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
			if len(paths) > 0 && len(existing.Paths) == 0 {
				existing.Paths = paths
			}
			if servesTLS(block.Body) {
				existing.SSL = true
				existing.Certificates = certificates
			}
		}
	}

	// A file croft wrote is one route, named after the file; any other name
	// in it is an alias of that route rather than a domain of its own.
	routes := make([]*entities.Route, 0, len(order))
	for _, domain := range order {
		route := merged[domain]
		if primary := r.primaryOf(route.File); primary != "" && primary != domain {
			if owner, ok := merged[primary]; ok && owner.File == route.File {
				owner.Aliases = append(owner.Aliases, entities.Alias{
					Domain: domain, SSL: route.SSL, Certificates: route.Certificates,
				})
				continue
			}
		}
		routes = append(routes, route)
	}
	for _, route := range routes {
		sort.Slice(route.Aliases, func(a, b int) bool { return route.Aliases[a].Domain < route.Aliases[b].Domain })
	}
	return routes, nil
}

// primaryOf is the domain a file in croft's own directory is named after, or
// nothing for a file somewhere else.
func (r *NginxRouteRepository) primaryOf(file string) string {
	if !strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(r.confDir)+"/") {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(filepath.ToSlash(file)), ".conf")
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
//
// A file that was there before is put back as it was, not removed: rewriting
// a domain that nginx then rejected used to delete the domain altogether.
func (r *NginxRouteRepository) walk(ctx context.Context, p plan.Plan) error {
	type previous struct {
		path    string
		content []byte
		existed bool
	}
	written := []previous{}

	for _, step := range p.Steps {
		before := previous{path: step.File}
		if step.IsFile() {
			if content, err := r.host.ReadFile(ctx, step.File); err == nil {
				before.content, before.existed = content, true
			}
		}

		err := host.RunStep(ctx, r.host, step)
		if err == nil {
			if step.IsFile() {
				written = append(written, before)
			}
			continue
		}

		if step.Optional {
			continue
		}

		for _, w := range written {
			if w.existed {
				_ = r.host.WriteFile(ctx, w.path, w.content, 0o644)
			} else {
				_ = r.host.RemoveFile(ctx, w.path)
			}
		}
		if len(written) > 0 {
			return fmt.Errorf("%w (the file was put back as it was, so nginx is unchanged)", err)
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
	argv := r.reload()
	_, err := r.host.Run(ctx, argv[0], argv[1:]...)
	return err
}

// reload is how this host restarts nginx. Alpine has no systemd, and a plan
// that ends in `systemctl reload nginx` there fails on its last step with the
// vhost already written — which is the worst place to stop.
//
// Asked here rather than hard-coded, so that the plan shows the command that
// will actually run on this machine.
func (r *NginxRouteRepository) reload() []string {
	if _, ok := r.host.Lookup(context.Background(), "systemctl"); ok {
		return []string{"systemctl", "reload", "nginx"}
	}
	return []string{"rc-service", "nginx", "reload"}
}

// acmeChallenge lets Let's Encrypt reach the token over port 80 without
// anything having to take the port away from nginx.
const acmeChallenge = `    location /.well-known/acme-challenge/ {
        root /var/lib/croft/acme;
    }
`

// render writes a route and its aliases. Names with a certificate redirect
// http to https and are served there, each with its own certificate; names
// without one — a domain without https, or an alias whose certificate is
// still to be issued — are served over http, which is also how the authority
// reaches them to issue one.
func render(route *entities.Route) string {
	secure, plain := []string{}, []string{}
	if route.SSL {
		secure = append(secure, route.Domain)
	} else {
		plain = append(plain, route.Domain)
	}
	for _, a := range route.Aliases {
		if a.SSL {
			secure = append(secure, a.Domain)
		} else {
			plain = append(plain, a.Domain)
		}
	}

	out := ""
	if len(plain) > 0 {
		out += fmt.Sprintf(`server {
    listen 80;
    server_name %s;

%s
%s}
`, strings.Join(plain, " "), acmeChallenge, locations(route))
	}
	if len(secure) > 0 {
		if out != "" {
			out += "\n"
		}
		out += fmt.Sprintf(`server {
    listen 80;
    server_name %s;

%s
    # Inside a location, not beside it: a return at server level runs before
    # nginx picks a location, and the challenge would be redirected too.
    location / {
        return 301 https://$host$request_uri;
    }
}
`, strings.Join(secure, " "), acmeChallenge)
	}
	if route.SSL {
		out += secureServer(route.Domain, route.CertDir(), route)
	}
	for _, a := range route.Aliases {
		if a.SSL {
			out += secureServer(a.Domain, a.CertDir(), route)
		}
	}
	return out
}

func secureServer(name, certificates string, route *entities.Route) string {
	return fmt.Sprintf(`
server {
    listen 443 ssl;
    server_name %s;

    ssl_certificate %s/fullchain.pem;
    ssl_certificate_key %s/privkey.pem;

%s}
`, name, certificates, certificates, locations(route))
}

// locations is the whole domain first, then each path of its own. nginx takes
// the longest prefix that matches wherever it is written, so the order is only
// for whoever reads the file — sorted, so the same route is always the same
// file and its hash does not change for nothing.
func locations(route *entities.Route) string {
	out := location("/", route.Target, route.Port, false)

	paths := append([]entities.PathRoute{}, route.Paths...)
	sort.Slice(paths, func(a, b int) bool { return paths[a].Prefix < paths[b].Prefix })
	for _, p := range paths {
		out += "\n" + location(p.Prefix, p.Target, p.Port, p.Strip)
	}
	return out
}

// location passes one prefix on. With strip, proxy_pass carries a URI of its
// own, "/", and nginx replaces the matched prefix with it: /api/tickets
// reaches the backend as /tickets.
func location(prefix, target string, port int, strip bool) string {
	uri := ""
	if strip {
		uri = "/"
	}

	return fmt.Sprintf(`    location %s {
        proxy_pass http://%s:%d%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # A websocket starts as HTTP and asks to switch protocol. nginx drops
        # the two headers that ask unless it is told to pass them on, and the
        # backend then refuses the upgrade — realtime falls back to polling,
        # or stops. HTTP/1.1 is what can carry the switch at all.
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $http_connection;

        # Anything that streams — progress while a plan runs, logs as they
        # arrive — comes event by event or not at all. Buffering holds it all
        # back until the work finishes, and the default read timeout would cut
        # a quiet connection off after a minute.
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }
`, prefix, target, port, uri)
}

// WritePlan is what adding a domain does, in the order it happens. Validating
// before reloading matters: a bad file that nginx accepts is a bug, a bad file
// that nginx rejects would take every other site down with it.
func (r *NginxRouteRepository) WritePlan(route *entities.Route) plan.Plan {
	return plan.New(
		plan.WriteFile("Write the vhost, with a hash of its own contents",
			r.vhost(route.Domain), marked(route)),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", r.reload()...),
	)
}

func (r *NginxRouteRepository) RemovePlan(domain string) plan.Plan {
	return plan.New(
		plan.Command("Remove the vhost", "rm", "-f", r.vhost(domain)),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", r.reload()...),
	)
}

// vhost is a path on the machine nginx runs on, which is always Linux. Joined
// by hand rather than with filepath, which would pick up the separator of
// whatever machine croft was compiled on and write a Windows path into a plan
// meant for a Linux host.
func (r *NginxRouteRepository) vhost(domain string) string {
	return r.confDir + "/" + domain + ".conf"
}

// marked renders the file with the header that lets us tell later whether
// somebody edited it by hand.
func marked(route *entities.Route) string {
	body := render(route)
	return fmt.Sprintf("%s\n%s%s\n# Edited by hand? This file will no longer be managed automatically.\n%s",
		Marker, hashKey, bodyHash(body), body)
}

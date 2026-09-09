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
	"regexp"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

const (
	Marker = "# managed-by: croft"
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

var (
	serverNamePattern = regexp.MustCompile(`server_name\s+([^;]+);`)
	proxyPassPattern  = regexp.MustCompile(`proxy_pass\s+https?://([0-9.]+):?(\d+)?`)
)

func (r *NginxRouteRepository) FindAll(ctx context.Context) ([]*entities.Route, error) {
	names, err := r.host.ListDir(ctx, r.confDir)
	if err != nil {
		return nil, err
	}

	routes := make([]*entities.Route, 0, len(names))
	for _, name := range names {
		if !strings.HasSuffix(name, ".conf") {
			continue
		}

		path := filepath.Join(r.confDir, name)
		content, err := r.host.ReadFile(ctx, path)
		if err != nil {
			continue
		}
		routes = append(routes, parse(path, strings.TrimSuffix(name, ".conf"), string(content)))
	}
	return routes, nil
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

func parse(path, fallbackDomain, content string) *entities.Route {
	domain := fallbackDomain
	if m := serverNamePattern.FindStringSubmatch(content); len(m) > 1 {
		domain = strings.Fields(m[1])[0]
	}

	target, port := "", 0
	if m := proxyPassPattern.FindStringSubmatch(content); len(m) > 1 {
		target = m[1]
		if len(m) > 2 && m[2] != "" {
			port, _ = strconv.Atoi(m[2])
		} else {
			port = 80
		}
	}

	return entities.NewRoute(entities.RouteProps{
		Domain:  domain,
		Target:  target,
		Port:    port,
		SSL:     strings.Contains(content, "ssl_certificate"),
		State:   stateOf(content),
		File:    path,
		Content: content,
	})
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
	body := render(route)
	content := fmt.Sprintf("%s\n%s%s\n# Edited by hand? This file will no longer be managed automatically.\n%s",
		Marker, hashKey, bodyHash(body), body)

	path := filepath.Join(r.confDir, route.Domain+".conf")
	return r.host.WriteFile(ctx, path, []byte(content), 0o644)
}

func (r *NginxRouteRepository) Remove(ctx context.Context, domain string) error {
	return r.host.RemoveFile(ctx, filepath.Join(r.confDir, domain+".conf"))
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

func render(route *entities.Route) string {
	if !route.SSL {
		return fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass http://%s:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, route.Domain, route.Target, route.Port)
	}

	return fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name %s;

    ssl_certificate /etc/letsencrypt/live/%s/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;

    location / {
        proxy_pass http://%s:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, route.Domain, route.Domain, route.Domain, route.Domain, route.Target, route.Port)
}

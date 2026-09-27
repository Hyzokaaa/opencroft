package agent

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/route/infrastructure/nginx"
)

// A site is a directory the container's own web server serves, and the one
// thing about it the machine does not say outright is where it came from: the
// served files are a build, and the code that built them lives somewhere else.
// install scripts copy dist/ into /var/www and move on.
//
// So croft asks the files. A checkout whose build output holds the very
// index.html being served — byte for byte — is the one that produced it. That
// is proof rather than a guess, and when nothing matches croft says so rather
// than picking the likeliest directory.

// searched is where checkouts are looked for. Deep enough for
// /opt/<app>/<part>/.git, shallow enough to answer in a moment.
var searched = []string{"/opt", "/srv", "/home", "/var/www", "/root", "/usr/local/src"}

// outputs are where builds conventionally land, in the order they are tried.
var outputs = []string{"dist", "build", "out"}

func (s *Server) inspectSite(ctx context.Context, name, domain string) (AdoptionDTO, error) {
	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		return AdoptionDTO{}, err
	}
	adopted := map[string]bool{}
	for _, service := range stored(config) {
		if adoption := adoptionFrom(config, service); adoption != nil && adoption.Site != "" {
			adopted[adoption.Site] = true
		}
	}

	_, sites := s.discover(ctx, name)
	var site *nginx.StaticSite
	for i := range sites {
		if slices.Contains(sites[i].Names, domain) && !adopted[sites[i].Root] {
			site = &sites[i]
		}
	}
	if site == nil {
		return AdoptionDTO{}, errors.New("the web server in " + name + " serves no directory for " + domain + " for croft to take on")
	}

	found := AdoptionDTO{Domains: site.Names, Site: site.Root}

	path, output := s.builtFrom(ctx, name, site.Root)
	if path == "" {
		found.Problem = "no git checkout on this container built what " + site.Root +
			" serves, so croft cannot tell where it comes from"
		return found, nil
	}
	found.Path, found.Output = path, output
	found.RunAs = s.owner(ctx, name, path)
	if s.exists(ctx, name, path+"/.env") {
		found.EnvFile = path + "/.env"
	}

	s.readCheckout(ctx, name, &found)
	return found, nil
}

// builtFrom finds the checkout whose build output holds exactly the index.html
// being served.
func (s *Server) builtFrom(ctx context.Context, name, root string) (string, string) {
	script := `want=$(sha256sum ` + root + `/index.html 2>/dev/null | cut -d' ' -f1); ` +
		`[ -n "$want" ] || exit 0; ` +
		`for g in $(find ` + strings.Join(searched, " ") + ` -maxdepth 4 -name .git -type d 2>/dev/null); do ` +
		`d=${g%/.git}; for o in ` + strings.Join(outputs, " ") + `; do ` +
		`if [ -f "$d/$o/index.html" ] && [ "$(sha256sum "$d/$o/index.html" | cut -d' ' -f1)" = "$want" ]; ` +
		`then echo "$d $o"; fi; done; done`

	out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "sh", "-c", script)
	if err != nil {
		return "", ""
	}

	for _, line := range strings.Split(out.Stdout, "\n") {
		path, output, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && pathPattern.MatchString(path) && slices.Contains(outputs, output) {
			return path, output
		}
	}
	return "", ""
}

func (s *Server) owner(ctx context.Context, name, path string) string {
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "stat", "-c", "%U", path)
	if err != nil {
		return ""
	}
	user := strings.TrimSpace(out.Stdout)
	if !userPattern.MatchString(user) {
		return ""
	}
	return user
}

func (s *Server) exists(ctx context.Context, name, path string) bool {
	_, err := s.host.Run(ctx, s.bin, "exec", name, "--", "test", "-f", path)
	return err == nil
}

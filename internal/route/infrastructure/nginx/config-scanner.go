package nginx

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// serverBlock is one `server { … }` found in the effective configuration,
// together with the file it came from.
type serverBlock struct {
	File string
	Body string
}

var fileMarker = regexp.MustCompile(`^# configuration file (.+):$`)

// scanServerBlocks walks the output of `nginx -T`, which concatenates every
// file nginx actually loaded, each preceded by a marker line.
//
// Reading the effective configuration rather than our own directory is what
// lets the panel see a server that was set up before it arrived.
func scanServerBlocks(dump string) []serverBlock {
	var blocks []serverBlock

	currentFile := ""
	lines := strings.Split(dump, "\n")

	// depth tracks nesting so that `server` inside `http` is found, but
	// `location` inside `server` is not mistaken for a new block.
	depth := 0
	capturing := false
	captureDepth := 0
	var body strings.Builder
	captureFile := ""

	for _, line := range lines {
		if m := fileMarker.FindStringSubmatch(strings.TrimSpace(line)); m != nil && !capturing {
			currentFile = m[1]
			continue
		}

		trimmed := strings.TrimSpace(line)
		code, _, _ := strings.Cut(trimmed, "#")
		code = strings.TrimSpace(code)

		opens := strings.Count(code, "{")
		closes := strings.Count(code, "}")

		startsServer := !capturing && isServerOpener(code)
		if startsServer {
			capturing = true
			captureDepth = depth
			captureFile = currentFile
			body.Reset()
		}

		if capturing {
			body.WriteString(line)
			body.WriteString("\n")
		}

		depth += opens - closes

		if capturing && depth <= captureDepth {
			blocks = append(blocks, serverBlock{File: captureFile, Body: body.String()})
			capturing = false
		}
	}

	return blocks
}

var serverOpener = regexp.MustCompile(`^server\s*\{`)

func isServerOpener(code string) bool {
	return serverOpener.MatchString(code)
}

var (
	serverNameDirective = regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)
	proxyPassDirective  = regexp.MustCompile(`(?m)^\s*proxy_pass\s+https?://([^;/\s]+)`)
	sslCertDirective    = regexp.MustCompile(`(?m)^\s*ssl_certificate\s`)
)

func serverNames(body string) []string {
	m := serverNameDirective.FindStringSubmatch(body)
	if m == nil {
		return nil
	}

	var names []string
	for _, name := range strings.Fields(m[1]) {
		// `_` is nginx's catch-all; it is not a domain anybody typed.
		if name != "_" {
			names = append(names, name)
		}
	}
	return names
}

// upstream returns the host and port a block proxies to, if it proxies at all.
// A block that only redirects to https has none, which is why blocks sharing a
// server_name are merged.
func upstream(body string) (string, int) {
	m := proxyPassDirective.FindStringSubmatch(body)
	if m == nil {
		return "", 0
	}

	target := m[1]
	host, port, found := strings.Cut(target, ":")
	if !found {
		return host, 80
	}

	number, err := strconv.Atoi(port)
	if err != nil {
		return host, 80
	}
	return host, number
}

func servesTLS(body string) bool {
	return sslCertDirective.MatchString(body)
}

var rootDirective = regexp.MustCompile(`(?m)^\s*root\s+([^;\s]+)\s*;`)

// StaticSite is a web server's own answer to "which directory is this domain":
// a server block that serves files from disk rather than passing requests on.
type StaticSite struct {
	Names []string
	Root  string
}

// StaticSites reads the server blocks in a `nginx -T` dump that serve files
// from a directory. Blocks sharing a name — the redirect on 80 and the site on
// 443 — are one site. A catch-all with no name of its own is left out: that is
// the distribution's welcome page, not a site anybody put there.
func StaticSites(dump string) []StaticSite {
	byRoot := map[string]*StaticSite{}
	order := []string{}

	for _, block := range scanServerBlocks(dump) {
		m := rootDirective.FindStringSubmatch(stripComments(block.Body))
		names := serverNames(block.Body)
		if m == nil || len(names) == 0 {
			continue
		}

		root := strings.TrimRight(m[1], "/")
		site, seen := byRoot[root]
		if !seen {
			site = &StaticSite{Root: root}
			byRoot[root] = site
			order = append(order, root)
		}
		for _, name := range names {
			if !slices.Contains(site.Names, name) {
				site.Names = append(site.Names, name)
			}
		}
	}

	sites := make([]StaticSite, 0, len(order))
	for _, root := range order {
		sites = append(sites, *byRoot[root])
	}
	return sites
}

// stripComments drops everything after a # on each line, so a commented-out
// root is not read as a live one.
func stripComments(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i], _, _ = strings.Cut(line, "#")
	}
	return strings.Join(lines, "\n")
}

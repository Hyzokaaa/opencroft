package acme

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Credentials for a DNS provider live in a file on the privileged side, never
// in the panel and never in the database the unprivileged half can read.
//
// certbot users already have these on disk, so those files are read first:
// somebody who has been issuing certificates for years should not have to
// re-enter anything.
const CredentialsPath = "/etc/croft/dns.conf"

var legacyPaths = map[string]string{
	"ovh":        "/etc/letsencrypt/ovh.ini",
	"cloudflare": "/etc/letsencrypt/cloudflare.ini",
}

// legoVariables maps our key names to the environment lego reads. Keys are
// accepted with or without certbot's `dns_` prefix.
var legoVariables = map[string]map[string]string{
	"ovh": {
		"ovh_endpoint":           "OVH_ENDPOINT",
		"ovh_application_key":    "OVH_APPLICATION_KEY",
		"ovh_application_secret": "OVH_APPLICATION_SECRET",
		"ovh_consumer_key":       "OVH_CONSUMER_KEY",
	},
	"cloudflare": {
		"cloudflare_api_token": "CLOUDFLARE_DNS_API_TOKEN",
		"cloudflare_email":     "CLOUDFLARE_EMAIL",
		"cloudflare_api_key":   "CLOUDFLARE_API_KEY",
	},
}

type Credentials struct {
	Provider string
	Values   map[string]string
	Source   string
}

// LoadCredentials finds a configured provider, or reports that none is.
func LoadCredentials() (*Credentials, error) {
	if found, err := readCroftFile(CredentialsPath); err == nil && found != nil {
		return found, nil
	}

	// Nothing of ours: fall back to what certbot left behind.
	providers := make([]string, 0, len(legacyPaths))
	for provider := range legacyPaths {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	for _, provider := range providers {
		path := legacyPaths[provider]
		values, err := readKeyValues(path)
		if err != nil || len(values) == 0 {
			continue
		}
		return &Credentials{Provider: provider, Values: values, Source: path}, nil
	}

	return nil, fmt.Errorf("no DNS credentials configured. Set them once with: croft dns set <ovh|cloudflare>")
}

func readCroftFile(path string) (*Credentials, error) {
	values, err := readKeyValues(path)
	if err != nil {
		return nil, err
	}

	provider := values["provider"]
	if provider == "" {
		return nil, nil
	}
	delete(values, "provider")

	return &Credentials{Provider: provider, Values: values, Source: path}, nil
}

// Apply puts the credentials where lego looks for them. They are set on this
// process only, and only for as long as it runs.
func (c *Credentials) Apply() error {
	mapping, ok := legoVariables[c.Provider]
	if !ok {
		return fmt.Errorf("croft does not know the DNS provider %q", c.Provider)
	}

	applied := 0
	for key, value := range c.Values {
		variable, ok := mapping[strings.TrimPrefix(key, "dns_")]
		if !ok || value == "" {
			continue
		}
		if err := os.Setenv(variable, value); err != nil {
			return err
		}
		applied++
	}

	if applied == 0 {
		return fmt.Errorf("%s has no usable %s credentials", c.Source, c.Provider)
	}
	return nil
}

// readKeyValues parses `key = value`, ignoring comments and blank lines.
//
// These files sit in /etc and are read by a process running as root. Sourcing
// them as shell — which is how the prototype did it — would execute whatever
// they contain.
func readKeyValues(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values, scanner.Err()
}

// Save writes credentials where only root can read them.
func Save(provider string, values map[string]string) error {
	if _, ok := legoVariables[provider]; !ok {
		return fmt.Errorf("croft does not know the DNS provider %q; it knows ovh and cloudflare", provider)
	}

	if err := os.MkdirAll(filepath.Dir(CredentialsPath), 0o700); err != nil {
		return err
	}

	var out strings.Builder
	out.WriteString("# Written by croft. These are secrets: keep the mode at 0600.\n")
	out.WriteString("provider = " + provider + "\n")

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if values[key] != "" {
			out.WriteString(key + " = " + values[key] + "\n")
		}
	}

	return os.WriteFile(CredentialsPath, []byte(out.String()), 0o600)
}

// Keys returns what a provider needs, for prompting.
func Keys(provider string) []string {
	switch provider {
	case "ovh":
		return []string{"ovh_endpoint", "ovh_application_key", "ovh_application_secret", "ovh_consumer_key"}
	case "cloudflare":
		return []string{"cloudflare_api_token"}
	default:
		return nil
	}
}

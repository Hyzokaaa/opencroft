// Package acme obtains certificates from Let's Encrypt.
//
// It replaces certbot: one less thing to install on the host, one less
// per-provider plugin to package, and one less program whose output has to be
// parsed to find out what happened.
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/registration"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
)

const (
	// WebRoot is where the HTTP-01 token is written. nginx serves it; the
	// generated vhosts carry the location block that points here.
	WebRoot = "/var/lib/croft/acme"
	// StoreDir keeps issued certificates, in the same shape certbot uses so
	// that reading them needs no special case.
	StoreDir = "/var/lib/croft/certificates"
)

type Issuer struct {
	webRoot  string
	storeDir string
	stateDir string
}

func NewIssuer() *Issuer {
	return &Issuer{
		webRoot:  WebRoot,
		storeDir: StoreDir,
		stateDir: "/var/lib/croft/acme",
	}
}

// account implements lego's user. The key identifies us to the authority and
// is kept between runs: a new key each time would register a new account and
// hit the registration rate limit soon enough.
type account struct {
	email        string
	key          crypto.PrivateKey
	registration *registration.Resource
}

func (a *account) GetEmail() string                        { return a.email }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }
func (a *account) GetRegistration() *registration.Resource { return a.registration }

func (i *Issuer) Issue(ctx context.Context, request services.IssueRequest, report func(string)) (*entities.Certificate, error) {
	if err := os.MkdirAll(filepath.Join(i.webRoot, ".well-known", "acme-challenge"), 0o755); err != nil {
		return nil, fmt.Errorf("preparing the challenge directory: %w", err)
	}

	directory := lego.LEDirectoryProduction
	environment := "production"
	if request.Staging {
		directory = lego.LEDirectoryStaging
		environment = "staging"
	}
	report(fmt.Sprintf("Talking to Let's Encrypt (%s)", environment))

	user, err := i.account(request.Email, request.Staging)
	if err != nil {
		return nil, err
	}

	config := lego.NewConfig(user)
	config.CADirURL = directory
	config.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, err
	}

	switch request.Challenge {
	case services.ChallengeDNS:
		report("Publishing a TXT record for the DNS challenge")
		provider, err := dnsProvider(os.Getenv("CROFT_DNS_PROVIDER"))
		if err != nil {
			return nil, err
		}
		if err := client.Challenge.SetDNS01Provider(provider); err != nil {
			return nil, err
		}
	default:
		// The token is written where nginx already serves it, so nothing has
		// to bind port 80 — nginx is already there.
		report("Serving the challenge from " + i.webRoot)
		if err := client.Challenge.SetHTTP01Provider(newWebRootProvider(i.webRoot)); err != nil {
			return nil, err
		}
	}

	if user.registration == nil {
		report("Registering an account")
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("registering with the authority: %w", err)
		}
		user.registration = reg
	}

	report("Asking for a certificate for " + request.Domain)
	issued, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{request.Domain},
		Bundle:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("obtaining the certificate: %w", err)
	}

	report("Storing it")
	path, err := i.store(request.Domain, issued)
	if err != nil {
		return nil, err
	}

	leaf, err := parseLeaf(issued.Certificate)
	if err != nil {
		return nil, err
	}

	return entities.NewCertificate(entities.CertificateProps{
		Domain:   request.Domain,
		Names:    leaf.DNSNames,
		Issuer:   issuerName(leaf),
		NotAfter: leaf.NotAfter,
		Path:     filepath.Join(path, "fullchain.pem"),
		Managed:  true,
	}), nil
}

// store writes the certificate in certbot's shape. Reading it back then needs
// no special case, and a person who knows certbot knows where to look.
func (i *Issuer) store(domain string, issued *certificate.Resource) (string, error) {
	dir := filepath.Join(i.storeDir, domain)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	files := map[string][]byte{
		"fullchain.pem": issued.Certificate,
		"privkey.pem":   issued.PrivateKey,
	}
	for name, content := range files {
		mode := os.FileMode(0o644)
		if name == "privkey.pem" {
			mode = 0o600
		}
		if err := os.WriteFile(filepath.Join(dir, name), content, mode); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// account loads the key from disk or creates one. Staging and production are
// separate accounts, because they are separate authorities.
func (i *Issuer) account(email string, staging bool) (*account, error) {
	name := "account.key"
	if staging {
		name = "account-staging.key"
	}
	path := filepath.Join(i.stateDir, name)

	if err := os.MkdirAll(i.stateDir, 0o700); err != nil {
		return nil, err
	}

	if raw, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(raw)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				return &account{email: email, key: key}, nil
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
		return nil, err
	}

	return &account{email: email, key: key}, nil
}

func parseLeaf(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("the authority returned something that is not a certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func issuerName(leaf *x509.Certificate) string {
	if len(leaf.Issuer.Organization) > 0 {
		return leaf.Issuer.Organization[0]
	}
	return leaf.Issuer.CommonName
}

// dnsProvider is deliberately a short list rather than lego's registry.
//
// The registry imports every provider it knows, which drags in the full SDK of
// every cloud in existence — tens of megabytes of binary to support two
// providers. Adding a third here is four lines; carrying all of them is not.
func dnsProvider(name string) (challenge.Provider, error) {
	switch name {
	case "ovh":
		return ovh.NewDNSProvider()
	case "cloudflare":
		return cloudflare.NewDNSProvider()
	case "":
		return nil, errors.New("set CROFT_DNS_PROVIDER to ovh or cloudflare, with that provider's credentials in the environment")
	default:
		return nil, fmt.Errorf("no DNS provider called %q; croft knows ovh and cloudflare", name)
	}
}

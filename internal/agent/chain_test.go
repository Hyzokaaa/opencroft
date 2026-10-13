package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"
)

type issued struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func issue(t *testing.T, name string, parent *issued, link string, ca bool, dns ...string) *issued {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  ca,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              dns,
	}
	if link != "" {
		template.IssuingCertificateURL = []string{link}
	}
	signer, signerKey := template, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &issued{cert, key}
}

// root → intermediate → leaf, the leaf saying where the intermediate is.
func aChain(t *testing.T) (root, intermediate, leaf *issued) {
	root = issue(t, "Root", nil, "", true)
	intermediate = issue(t, "Intermediate", root, "", true)
	leaf = issue(t, "gitlab.example.com", intermediate, "http://example.com/intermediate", false, "gitlab.example.com")
	return
}

func complete(t *testing.T, roots *x509.CertPool, sent []*x509.Certificate, links map[string]*x509.Certificate) error {
	t.Helper()
	_, err := completeChain(context.Background(), "https://gitlab.example.com/core/backend.git", roots,
		func(context.Context, string, string) ([]*x509.Certificate, error) { return sent, nil },
		func(_ context.Context, link string) (*x509.Certificate, error) {
			if cert, ok := links[link]; ok {
				return cert, nil
			}
			return nil, errors.New("not found")
		})
	return err
}

// What a browser fetches is what the container is given, and nothing else.
func TestALeftOutIntermediateIsFetchedFromWhereTheCertificateSays(t *testing.T) {
	root, intermediate, leaf := aChain(t)
	roots := x509.NewCertPool()
	roots.AddCert(root.cert)

	missing, err := completeChain(context.Background(), "https://gitlab.example.com/core/backend.git", roots,
		func(_ context.Context, address, name string) ([]*x509.Certificate, error) {
			if address != "gitlab.example.com:443" || name != "gitlab.example.com" {
				t.Errorf("asked %s for %s", address, name)
			}
			return []*x509.Certificate{leaf.cert}, nil
		},
		func(_ context.Context, link string) (*x509.Certificate, error) {
			if link != "http://example.com/intermediate" {
				t.Errorf("followed %s", link)
			}
			return intermediate.cert, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if missing == nil || len(missing.Certificates) != 1 || missing.Certificates[0].Name != "Intermediate" {
		t.Fatalf("missing is %+v", missing)
	}
	if missing.Host != "gitlab.example.com" || missing.Root != "Root" {
		t.Errorf("missing is %+v", missing)
	}
}

func TestAServerThatSendsItsChainNeedsNothing(t *testing.T) {
	root, intermediate, leaf := aChain(t)
	roots := x509.NewCertPool()
	roots.AddCert(root.cert)

	missing, err := completeChain(context.Background(), "https://gitlab.example.com/x.git", roots,
		func(context.Context, string, string) ([]*x509.Certificate, error) {
			return []*x509.Certificate{leaf.cert, intermediate.cert}, nil
		},
		func(context.Context, string) (*x509.Certificate, error) {
			t.Error("fetched a certificate nobody needed")
			return nil, errors.New("no")
		})
	if err != nil || missing != nil {
		t.Errorf("missing %+v, err %v", missing, err)
	}
}

// Whatever the links lead to, nothing is trusted that does not end at a root
// the host already trusts.
func TestAChainToAnUntrustedRootIsNotCompleted(t *testing.T) {
	_, intermediate, leaf := aChain(t)
	other := issue(t, "Other root", nil, "", true)
	roots := x509.NewCertPool()
	roots.AddCert(other.cert)

	if err := complete(t, roots, []*x509.Certificate{leaf.cert},
		map[string]*x509.Certificate{"http://example.com/intermediate": intermediate.cert}); err == nil {
		t.Error("a chain to a root nobody trusts was completed")
	}
}

// A certificate is only good for its own name.
func TestAChainForAnotherNameIsNotCompleted(t *testing.T) {
	root, intermediate, _ := aChain(t)
	elsewhere := issue(t, "elsewhere.example.com", intermediate, "http://example.com/intermediate", false,
		"elsewhere.example.com")
	roots := x509.NewCertPool()
	roots.AddCert(root.cert)

	if err := complete(t, roots, []*x509.Certificate{elsewhere.cert},
		map[string]*x509.Certificate{"http://example.com/intermediate": intermediate.cert}); err == nil {
		t.Error("a certificate for another name was taken")
	}
}

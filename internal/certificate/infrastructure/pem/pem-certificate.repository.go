// Package pem reads certificates from the files on disk.
package pem

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"path"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// Where certificates live on a typical host. certbot owns the first; the
// second is ours. Both are read, because a certificate somebody else issued is
// still a certificate that expires.
var searchPaths = []string{
	"/etc/letsencrypt/live",
	"/var/lib/croft/certificates",
}

type PEMCertificateRepository struct {
	host  host.Host
	paths []string
	ours  string
}

func NewPEMCertificateRepository(h host.Host) *PEMCertificateRepository {
	return &PEMCertificateRepository{
		host:  h,
		paths: searchPaths,
		ours:  "/var/lib/croft/certificates",
	}
}

func (r *PEMCertificateRepository) FindAll(ctx context.Context) ([]*entities.Certificate, error) {
	found := []*entities.Certificate{}
	seen := map[string]bool{}

	for _, root := range r.paths {
		entries, err := r.host.ListDir(ctx, root)
		if err != nil {
			continue // a missing directory is not an error, it is an absence
		}

		for _, name := range entries {
			// certbot keeps a README beside the domain directories.
			if strings.HasPrefix(name, ".") || name == "README" {
				continue
			}

			file := path.Join(root, name, "fullchain.pem")
			raw, err := r.host.ReadFile(ctx, file)
			if err != nil {
				continue
			}

			certificate := parse(raw, file, strings.HasPrefix(root, r.ours))
			if certificate == nil || seen[certificate.Domain] {
				continue
			}
			seen[certificate.Domain] = true
			found = append(found, certificate)
		}
	}
	return found, nil
}

// parse reads the leaf certificate: the first block in a fullchain file is the
// one that actually answers for the domain.
func parse(raw []byte, file string, ours bool) *entities.Certificate {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}

	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	domain := leaf.Subject.CommonName
	if domain == "" && len(leaf.DNSNames) > 0 {
		domain = leaf.DNSNames[0]
	}

	issuer := leaf.Issuer.Organization
	name := leaf.Issuer.CommonName
	if len(issuer) > 0 {
		name = issuer[0]
	}

	return entities.NewCertificate(entities.CertificateProps{
		Domain:     domain,
		Names:      leaf.DNSNames,
		Issuer:     name,
		NotAfter:   leaf.NotAfter,
		Path:       file,
		Managed:    ours,
		SelfSigned: leaf.Issuer.String() == leaf.Subject.String(),
	})
}

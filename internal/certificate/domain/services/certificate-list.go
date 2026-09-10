package services

import (
	"context"
	"sort"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/repositories"
)

type ListCertificates struct {
	certificates repositories.CertificateRepository
}

func NewListCertificates(certificates repositories.CertificateRepository) *ListCertificates {
	return &ListCertificates{certificates: certificates}
}

// Execute puts whatever expires soonest first: that is the one you need to
// know about.
func (s *ListCertificates) Execute(ctx context.Context) ([]*entities.Certificate, error) {
	found, err := s.certificates.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(found, func(a, b int) bool {
		return found[a].NotAfter.Before(found[b].NotAfter)
	})
	return found, nil
}

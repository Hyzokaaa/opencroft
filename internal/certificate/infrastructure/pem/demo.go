package pem

import (
	"context"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
)

// DemoCertificateRepository shows the cases worth seeing: one healthy, one
// about to expire, and one that signed itself.
type DemoCertificateRepository struct{}

func NewDemoCertificateRepository() *DemoCertificateRepository {
	return &DemoCertificateRepository{}
}

func (r *DemoCertificateRepository) FindAll(_ context.Context) ([]*entities.Certificate, error) {
	now := time.Now()

	return []*entities.Certificate{
		entities.NewCertificate(entities.CertificateProps{
			Domain: "soporte.example.com", Names: []string{"soporte.example.com"},
			Issuer: "Let's Encrypt", NotAfter: now.AddDate(0, 0, 68),
			Path: "/etc/letsencrypt/live/soporte.example.com/fullchain.pem", Managed: true,
		}),
		entities.NewCertificate(entities.CertificateProps{
			Domain: "www.example.com", Names: []string{"www.example.com", "example.com"},
			Issuer: "Let's Encrypt", NotAfter: now.AddDate(0, 0, 9),
			Path: "/etc/letsencrypt/live/www.example.com/fullchain.pem", Managed: true,
		}),
		entities.NewCertificate(entities.CertificateProps{
			Domain: "old.example.com", Names: []string{"old.example.com"},
			Issuer: "old.example.com", NotAfter: now.AddDate(1, 0, 0),
			Path: "/etc/nginx/ssl/old.example.com.pem", SelfSigned: true,
		}),
	}, nil
}

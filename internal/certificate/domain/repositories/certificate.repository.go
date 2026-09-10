package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
)

// CertificateRepository reads the certificate files themselves. There is no
// database of certificates: the PEM on disk knows when it expires, and asking
// it is both simpler and impossible to get out of sync.
type CertificateRepository interface {
	FindAll(ctx context.Context) ([]*entities.Certificate, error)
}

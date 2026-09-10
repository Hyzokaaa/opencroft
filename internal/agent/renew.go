package agent

import (
	"context"
	"log"
	"time"

	certificateEntities "github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
)

const (
	// Let's Encrypt issues for 90 days and expects renewal at 30. Renewing
	// earlier than that wastes rate limit; later leaves no room to fix a
	// failure before it becomes an outage.
	RenewBelowDays = 30
	// Twice a day. A certificate has a month of slack, so the only thing
	// frequency buys is recovering sooner from a temporary failure.
	RenewEvery = 12 * time.Hour
)

// Renew keeps certificates alive without anybody remembering to.
//
// Only certificates croft issued are touched. certbot has its own timer for
// the ones it owns, and two programs renewing the same certificate is how you
// exhaust a rate limit.
func (s *Server) Renew(ctx context.Context) {
	// A moment after boot, so a host that was off for a month catches up
	// without waiting half a day.
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.renewOnce(ctx)
			timer.Reset(RenewEvery)
		}
	}
}

func (s *Server) renewOnce(ctx context.Context) {
	found, err := s.certificates.FindAll(ctx)
	if err != nil {
		log.Printf("renewal: could not read the certificates: %v", err)
		return
	}

	now := time.Now()
	for _, certificate := range found {
		if !s.shouldRenew(certificate, now) {
			continue
		}

		log.Printf("renewal: %s expires in %d days, renewing",
			certificate.Domain, certificate.DaysLeft(now))

		if err := s.renew(ctx, certificate.Domain); err != nil {
			// Left for the next run. The panel is already warning about the
			// expiry, and a failure here does not make things worse.
			log.Printf("renewal: %s failed: %v", certificate.Domain, err)
			continue
		}
		log.Printf("renewal: %s renewed", certificate.Domain)
	}
}

func (s *Server) shouldRenew(certificate *certificateEntities.Certificate, now time.Time) bool {
	if !certificate.Managed {
		return false // certbot's, or somebody else's
	}
	return certificate.DaysLeft(now) < RenewBelowDays
}

func (s *Server) renew(ctx context.Context, domain string) error {
	issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())

	if _, err := issuer.Execute(ctx, certificateServices.IssueRequest{
		Domain:    domain,
		Challenge: chooseChallenge(),
	}, nil); err != nil {
		return err
	}

	// nginx holds the old certificate open until it is told to look again.
	// The file changed underneath it, so the reload is the point.
	return s.routes.Reload(ctx)
}

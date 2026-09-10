package entities

import "time"

// Certificate is a TLS certificate found on the host, whoever issued it.
type Certificate struct {
	Domain     string
	Names      []string
	Issuer     string
	NotAfter   time.Time
	Path       string
	Managed    bool
	SelfSigned bool
}

type CertificateProps struct {
	Domain     string
	Names      []string
	Issuer     string
	NotAfter   time.Time
	Path       string
	Managed    bool
	SelfSigned bool
}

func NewCertificate(props CertificateProps) *Certificate {
	return &Certificate{
		Domain:     props.Domain,
		Names:      props.Names,
		Issuer:     props.Issuer,
		NotAfter:   props.NotAfter,
		Path:       props.Path,
		Managed:    props.Managed,
		SelfSigned: props.SelfSigned,
	}
}

// DaysLeft is negative once the certificate has expired.
func (c *Certificate) DaysLeft(now time.Time) int {
	return int(c.NotAfter.Sub(now).Hours() / 24)
}

func (c *Certificate) Expired(now time.Time) bool {
	return now.After(c.NotAfter)
}

// Renewal is due well before expiry: Let's Encrypt issues for 90 days and
// expects renewal at 30, so 21 days left already means something is wrong with
// whatever should have renewed it.
const RenewalWindow = 21

func (c *Certificate) NeedsAttention(now time.Time) bool {
	return c.DaysLeft(now) < RenewalWindow
}

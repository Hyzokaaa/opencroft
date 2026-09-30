package services

import (
	"context"
	"errors"
	"regexp"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
)

var (
	ErrDomainRequired   = errors.New("a domain is required")
	ErrDomainInvalid    = errors.New("that does not look like a domain name")
	ErrEmailInvalid     = errors.New("that does not look like an email address")
	ErrWildcardNeedsDNS = errors.New("a wildcard certificate can only be proved over DNS: set the DNS credentials first")
)

var (
	domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
	emailPattern  = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// Issuer is the port to a certificate authority. The domain says what it
// wants; ACME, its account key and its challenges are infrastructure.
type Issuer interface {
	// Issue blocks until the authority answers. Report narrates the wait,
	// which can be a minute and is otherwise silent.
	Issue(ctx context.Context, request IssueRequest, report func(string)) (*entities.Certificate, error)
}

type Challenge string

const (
	// ChallengeHTTP proves control by serving a file the authority fetches
	// over port 80. It needs no credentials, which is why it is the default.
	ChallengeHTTP Challenge = "http-01"
	// ChallengeDNS proves control by publishing a TXT record. It needs
	// credentials for the DNS provider, and is the only way to get a
	// wildcard.
	ChallengeDNS Challenge = "dns-01"
)

type IssueRequest struct {
	Domain    string
	Email     string
	Challenge Challenge

	// Wildcard asks for *.Domain and Domain itself in one certificate, so every
	// subdomain has https the moment it is added, with nothing to issue.
	Wildcard bool

	// Staging points at Let's Encrypt's test environment, whose certificates
	// browsers reject. Production limits five failed validations per account
	// per hour and five duplicate certificates per week, and burning those
	// locks a real domain out for days — so integrations are proven here.
	Staging bool
}

type IssueCertificate struct {
	issuer Issuer
}

func NewIssueCertificate(issuer Issuer) *IssueCertificate {
	return &IssueCertificate{issuer: issuer}
}

func (s *IssueCertificate) Execute(ctx context.Context, request IssueRequest, report func(string)) (*entities.Certificate, error) {
	if request.Domain == "" {
		return nil, ErrDomainRequired
	}
	if !domainPattern.MatchString(request.Domain) {
		return nil, ErrDomainInvalid
	}
	if request.Email != "" && !emailPattern.MatchString(request.Email) {
		return nil, ErrEmailInvalid
	}
	if request.Wildcard && request.Challenge == "" {
		request.Challenge = ChallengeDNS
	}
	if request.Wildcard && request.Challenge != ChallengeDNS {
		return nil, ErrWildcardNeedsDNS
	}
	if request.Challenge == "" {
		request.Challenge = ChallengeHTTP
	}

	if report == nil {
		report = func(string) {}
	}
	return s.issuer.Issue(ctx, request, report)
}

// StoreName is the directory a certificate is kept in. A wildcard's starts
// with "_.", which no hostname can, so it never collides with a certificate
// for a subdomain that happens to be called "wildcard".
func StoreName(domain string, wildcard bool) string {
	if wildcard {
		return "_." + domain
	}
	return domain
}

// Names is what the authority is asked to vouch for.
func (r IssueRequest) Names() []string {
	if r.Wildcard {
		return []string{"*." + r.Domain, r.Domain}
	}
	return []string{r.Domain}
}

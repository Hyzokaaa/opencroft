package services

import (
	"context"
	"errors"
	"regexp"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
)

var (
	ErrDomainRequired = errors.New("a domain is required")
	ErrDomainInvalid  = errors.New("that does not look like a domain name")
	ErrEmailInvalid   = errors.New("that does not look like an email address")
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
	if request.Challenge == "" {
		request.Challenge = ChallengeHTTP
	}

	if report == nil {
		report = func(string) {}
	}
	return s.issuer.Issue(ctx, request, report)
}

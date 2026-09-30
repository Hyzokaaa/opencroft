package services

import (
	"context"
	"errors"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
)

type recordingIssuer struct{ got IssueRequest }

func (r *recordingIssuer) Issue(_ context.Context, request IssueRequest, _ func(string)) (*entities.Certificate, error) {
	r.got = request
	return nil, nil
}

// A wildcard asks for the name and everything one label below, is kept apart
// from any subdomain's own certificate, and is proved over DNS or not at all.
func TestAWildcardAsksForBothNamesOverDNS(t *testing.T) {
	issuer := &recordingIssuer{}
	if _, err := NewIssueCertificate(issuer).Execute(context.Background(),
		IssueRequest{Domain: "example.com", Wildcard: true}, nil); err != nil {
		t.Fatal(err)
	}
	if issuer.got.Challenge != ChallengeDNS {
		t.Errorf("proved over %s", issuer.got.Challenge)
	}
	if names := issuer.got.Names(); len(names) != 2 || names[0] != "*.example.com" || names[1] != "example.com" {
		t.Errorf("asked for %v", names)
	}
	if StoreName("example.com", true) != "_.example.com" || StoreName("example.com", false) != "example.com" {
		t.Error("a wildcard is kept where a subdomain's certificate could be")
	}

	_, err := NewIssueCertificate(issuer).Execute(context.Background(),
		IssueRequest{Domain: "example.com", Wildcard: true, Challenge: ChallengeHTTP}, nil)
	if !errors.Is(err, ErrWildcardNeedsDNS) {
		t.Errorf("over http: %v", err)
	}
}

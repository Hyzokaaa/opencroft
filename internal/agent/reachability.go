package agent

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
)

// Before asking Let's Encrypt for a certificate over http, croft asks itself
// the same question the authority will: does http://<domain> reach this
// server? It leaves a file where the challenge would be and fetches it by
// the domain's name. Following the domain the way a visitor would means it
// is right behind a proxy or a NAT too, where comparing addresses would not.
//
// The answer decides whether to ask the authority at all. A customer whose
// DNS does not point here yet is the normal case, not an error — and asking
// regardless spends attempts Let's Encrypt limits per hour.

// RetryEvery is how often names waiting for their DNS are looked at again.
const RetryEvery = 5 * time.Minute

type reachFunc func(ctx context.Context, domain string) error

// waiting remembers why a name is still served over http, for the panel.
type waiting struct {
	mu      sync.Mutex
	reasons map[string]string
}

func (w *waiting) set(domain, reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reasons == nil {
		w.reasons = map[string]string{}
	}
	w.reasons[domain] = reason
}

func (w *waiting) clear(domain string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.reasons, domain)
}

func (w *waiting) reason(domain string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reasons[domain]
}

// selfCheck is the real question, asked over the network.
func (s *Server) selfCheck(ctx context.Context, domain string) error {
	if _, err := net.DefaultResolver.LookupHost(ctx, domain); err != nil {
		return fmt.Errorf("%s has no DNS record yet — it needs an A record pointing to this server", domain)
	}

	token := make([]byte, 16)
	_, _ = rand.Read(token)
	name := "croft-check-" + hex.EncodeToString(token)
	file := acme.WebRoot + "/.well-known/acme-challenge/" + name
	if err := s.host.WriteFile(ctx, file, []byte(name), 0o644); err != nil {
		return fmt.Errorf("could not prepare the check: %w", err)
	}
	defer func() { _ = s.host.RemoveFile(context.Background(), file) }()

	client := &http.Client{
		Timeout: 10 * time.Second,
		// A proxy in front may send http to https; following it is what
		// the authority does too. Only the token is checked, not the
		// certificate, which is the thing about to be obtained.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+domain+"/.well-known/acme-challenge/"+name, nil)
	res, err := client.Do(req)
	if err != nil {
		addresses, _ := net.DefaultResolver.LookupHost(ctx, domain)
		return fmt.Errorf("%s points to %s, which did not answer for it: it has to point to this server",
			domain, strings.Join(addresses, ", "))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 256))
	if strings.TrimSpace(string(body)) != name {
		return fmt.Errorf("%s answers, but not from this server — its DNS points somewhere else", domain)
	}
	return nil
}

func (s *Server) reaches(ctx context.Context, domain string) error {
	if s.reachCheck != nil {
		return s.reachCheck(ctx, domain)
	}
	return s.selfCheck(ctx, domain)
}

// RetryWaiting looks again, every few minutes, at the names that are served
// over http only because their DNS did not point here yet, and gives each its
// certificate the moment it does. Nobody has to come back and press anything.
func (s *Server) RetryWaiting(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.retryWaitingOnce(ctx)
			timer.Reset(RetryEvery)
		}
	}
}

func (s *Server) retryWaitingOnce(ctx context.Context) {
	routes, err := s.routes.FindAll(ctx)
	if err != nil {
		return
	}
	for _, route := range routes {
		if route.State != routeEnums.StateManaged || !route.SSL {
			continue
		}
		for _, alias := range route.Aliases {
			if alias.SSL {
				continue
			}
			if err := s.certifyAlias(ctx, route.Domain, alias.Domain, nil); err != nil {
				log.Printf("waiting: %s: %v", alias.Domain, err)
			}
		}
	}
}

// certifyAlias gives a name served over http its certificate and moves it to
// https — if it reaches this server; otherwise it records why not.
func (s *Server) certifyAlias(ctx context.Context, domain, alias string, report func(string)) error {
	if report == nil {
		report = func(string) {}
	}
	if err := s.reaches(ctx, alias); err != nil {
		s.waiting.set(alias, err.Error())
		return errWaiting{err}
	}

	route, _, err := s.owned(ctx, domain)
	if err != nil {
		return err
	}
	certified := routeEntities.Alias{Domain: alias, SSL: true}
	if wildcard, covered, err := s.wildcardFor(ctx, alias); err == nil && covered {
		certified.Certificates = wildcard
	} else {
		issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())
		if _, err := issuer.Execute(ctx, certificateServices.IssueRequest{
			Domain: alias, Challenge: certificateServices.ChallengeHTTP,
		}, report); err != nil {
			s.waiting.set(alias, "Let's Encrypt refused the certificate: "+err.Error())
			return err
		}
	}
	if err := s.routes.Write(ctx, route.WithAlias(certified)); err != nil {
		return err
	}
	s.waiting.clear(alias)
	return nil
}

// errWaiting is a name that does not reach this server yet: expected, and
// handled by waiting rather than by failing.
type errWaiting struct{ cause error }

func (e errWaiting) Error() string { return e.cause.Error() }

func isWaiting(err error) bool {
	var w errWaiting
	return errors.As(err, &w)
}

// withWaiting says, for each name still served over http, why.
func (s *Server) withWaiting(aliases []AliasDTO) []AliasDTO {
	for i := range aliases {
		if !aliases[i].SSL {
			aliases[i].Waiting = s.waiting.reason(aliases[i].Domain)
		}
	}
	return aliases
}

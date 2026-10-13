package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/deploy/infrastructure/container"
)

// A server that sends its certificate without the intermediates it hangs from
// works in a browser and fails in git. Browsers forgive it: every certificate
// says where its issuer can be downloaded (its "authority information
// access"), and they go and get it. git does not, and says only that the
// certificate could not be verified.
//
// croft does what the browser does, once, while the plan is made: it asks the
// repository's server for its certificates, and when they do not lead to a
// root this host trusts, it follows those links until they do. What it had to
// fetch is what the container is then told to trust, in a step of the plan
// that names it. Nothing is trusted that does not lead, for that very name,
// to a root the host already trusts — so nothing new is trusted at all; the
// container is only given the missing links.

// missingFunc returns what the repository's server leaves out, or nil when it
// sends all it should.
type missingFunc func(ctx context.Context, repo string) (*container.Missing, error)

// Patience for the server and for each certificate fetched: a plan waits on
// them, and a server that does not answer is for the fetch to report.
const chainPatience = 5 * time.Second

// missing is what the repository's server leaves out. nil s.missingCheck asks
// the network.
func (s *Server) missing(ctx context.Context, repo string) (*container.Missing, error) {
	if s.missingCheck != nil {
		return s.missingCheck(ctx, repo)
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	return completeChain(ctx, repo, roots, presented, issuer)
}

// completeChain follows the links from the last certificate the server sent
// up to a root in roots, and returns the certificates it had to fetch.
func completeChain(
	ctx context.Context,
	repo string,
	roots *x509.CertPool,
	present func(ctx context.Context, address, name string) ([]*x509.Certificate, error),
	fetch func(ctx context.Context, url string) (*x509.Certificate, error),
) (*container.Missing, error) {
	parsed, err := url.Parse(repo)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return nil, nil
	}
	name, port := parsed.Hostname(), parsed.Port()
	if port == "" {
		port = "443"
	}

	sent, err := present(ctx, net.JoinHostPort(name, port), name)
	if err != nil {
		return nil, err
	}
	if len(sent) == 0 {
		return nil, fmt.Errorf("%s sent no certificate", name)
	}

	leaf := sent[0]
	intermediates := x509.NewCertPool()
	for _, cert := range sent[1:] {
		intermediates.AddCert(cert)
	}
	root := ""
	verifies := func() bool {
		chains, err := leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: roots, Intermediates: intermediates})
		if err != nil {
			return false
		}
		root = chains[0][len(chains[0])-1].Subject.CommonName
		return true
	}
	if verifies() {
		return nil, nil
	}

	// Four links is more than any public chain has; a loop or a chain that
	// never ends stops here.
	fetched := []*x509.Certificate{}
	for current := sent[len(sent)-1]; len(fetched) < 4 && len(current.IssuingCertificateURL) > 0; {
		next, err := fetch(ctx, current.IssuingCertificateURL[0])
		if err != nil {
			return nil, fmt.Errorf("%s leaves out the certificate it hangs from, and it could not be fetched: %w", name, err)
		}
		fetched = append(fetched, next)
		intermediates.AddCert(next)
		if verifies() {
			missing := &container.Missing{Host: name, Root: root}
			for _, cert := range fetched {
				missing.Certificates = append(missing.Certificates, container.Certificate{
					Name: cert.Subject.CommonName,
					PEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}),
				})
			}
			return missing, nil
		}
		current = next
	}
	return nil, fmt.Errorf("%s presents a certificate that does not lead to a root this server trusts", name)
}

// presented is what the server sends, read without judging it: judging is
// completeChain's job.
func presented(ctx context.Context, address, name string) ([]*x509.Certificate, error) {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: chainPatience},
		// Only reading what the server sends: nothing is sent over this
		// connection, and the certificates are verified right after.
		Config: &tls.Config{ServerName: name, InsecureSkipVerify: true},
	}
	ctx, cancel := context.WithTimeout(ctx, chainPatience)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.(*tls.Conn).ConnectionState().PeerCertificates, nil
}

// issuer downloads a certificate from where another one says it is. It is
// usually plain http, which is safe: a certificate fetched is only kept if
// it leads to a trusted root.
func issuer(ctx context.Context, link string) (*x509.Certificate, error) {
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return nil, fmt.Errorf("%s is not a link croft follows", link)
	}
	ctx, cancel := context.WithTimeout(ctx, chainPatience)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", link, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return parseCertificate(body)
}

// parseCertificate reads one certificate, in DER — what the links serve — or
// PEM.
func parseCertificate(body []byte) (*x509.Certificate, error) {
	if block, _ := pem.Decode(body); block != nil {
		body = block.Bytes
	}
	cert, err := x509.ParseCertificate(body)
	if err != nil {
		return nil, errors.New("what came back is not a certificate")
	}
	return cert, nil
}

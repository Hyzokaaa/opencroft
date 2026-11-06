package agent

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/route/infrastructure/nginx"
)

// A site has no process of its own, so "is it running" has no answer: the
// web server can be up and serving a 404, or a 502 from a location it proxies.
// What can be known is what a visitor would get, so croft asks for the page.
//
// ServingDTO is that answer, read when the container is listed and never
// remembered. Status is the HTTP code that came back; Problem is set instead
// when nothing answered at all.
type ServingDTO struct {
	Domain  string    `json:"domain"`
	Status  int       `json:"status,omitempty"`
	Problem string    `json:"problem,omitempty"`
	At      time.Time `json:"at"`
}

type servingFunc func(ctx context.Context, address string, port int, domain string) ServingDTO

// sitesServing asks each adopted site for its front page, all at once: a
// container with three sites waits for the slowest, not for the sum.
func (s *Server) sitesServing(ctx context.Context, name string, out []ServiceDTO, served []nginx.StaticSite) {
	domains := map[string]string{}
	for _, site := range served {
		for _, domain := range site.Names {
			if domainPattern.MatchString(domain) {
				domains[site.Root] = domain
				break
			}
		}
	}

	var wanted []int
	for i := range out {
		if out[i].Adopted != nil && domains[out[i].Adopted.Site] != "" {
			wanted = append(wanted, i)
		}
	}
	if len(wanted) == 0 {
		return
	}

	// A stopped container is not asked: nothing would answer, and saying
	// "no answer" would read as a fault of the site rather than of the
	// container being off, which the page already says.
	instance, err := s.instances.FindByName(ctx, name)
	if err != nil || instance == nil || instance.Address == "" || instance.Status != enums.StatusRunning {
		return
	}

	check := s.servingCheck
	if check == nil {
		check = askSite
	}

	var wg sync.WaitGroup
	for _, i := range wanted {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			answer := check(ctx, instance.Address, 80, domains[out[i].Adopted.Site])
			out[i].Serving = &answer
		}(i)
	}
	wg.Wait()
}

// askSite requests the site's front page from the container directly, with
// the site's name, the way nginx in front of it would. It goes around the
// public DNS on purpose: behind Cloudflare, or with the name resolved
// differently on this host, asking by name would test something else.
//
// A redirect is followed only while it stays on the same name — the usual
// http-to-https one — and still to the container. The certificate is not
// checked here: whether it is valid is what the certificates page says, and
// mixing the two would report a site as down because its certificate is.
func askSite(ctx context.Context, address string, port int, domain string) ServingDTO {
	answer := ServingDTO{Domain: domain, At: time.Now().UTC()}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, to, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(address, to))
			},
			TLSClientConfig:   &tls.Config{ServerName: domain, InsecureSkipVerify: true}, //nolint:gosec // see above
			DisableKeepAlives: true,
			Proxy:             nil,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Hostname() != domain {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	url := "http://" + domain + "/"
	if port != 80 {
		url = "http://" + net.JoinHostPort(domain, strconv.Itoa(port)) + "/"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		answer.Problem = "croft could not build a request for " + domain
		return answer
	}

	response, err := client.Do(request)
	if err != nil {
		answer.Problem = "nothing answered for " + domain + " on the container's web server"
		return answer
	}
	_ = response.Body.Close()
	answer.Status = response.StatusCode
	return answer
}

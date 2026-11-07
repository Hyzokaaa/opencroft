// Package reconcile compares what the system reports with what the resources
// say they want. The difference is drift — and drift is information shown to
// the user, never a change applied on their behalf.
package reconcile

import (
	"fmt"
	"path"
	"strings"
	"time"

	certificates "github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	instances "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceEnums "github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	routes "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Kind     string   `json:"kind"`
	Subject  string   `json:"subject"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

// Inspect never mutates anything. It reports.
func Inspect(is []*instances.Instance, rs []*routes.Route, cs []*certificates.Certificate) []Finding {
	now := time.Now()
	findings := []Finding{}

	byAddress := map[string]*instances.Instance{}
	byDomain := map[string]*instances.Instance{}
	for _, i := range is {
		if i.Address != "" {
			byAddress[i.Address] = i
		}
		if i.Domain != "" {
			byDomain[i.Domain] = i
		}
	}

	for _, route := range rs {
		switch route.State {
		case routeEnums.StateAdopted:
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Kind:     "edited-by-hand",
				Subject:  route.Domain,
				Message:  "This file was edited after it was generated.",
				Hint:     "It is left exactly as you wrote it and will not be overwritten.",
			})
		case routeEnums.StateUnmanaged:
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Kind:     "unmanaged",
				Subject:  route.Domain,
				Message:  "This route was not created by OpenCroft.",
				Hint:     "croft lists it and warns about its certificate, but will not change it.",
			})
		}

		// A route to the loopback is the panel itself, not a container that
		// went missing. Without this the panel accuses itself.
		if strings.HasPrefix(route.Target, "127.") {
			continue
		}

		target, known := byAddress[route.Target]
		if !known {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     "orphan-route",
				Subject:  route.Domain,
				Message:  fmt.Sprintf("Routes to %s, where no container lives.", route.Target),
				Hint:     "The container was removed, or its address changed.",
			})
			continue
		}

		// A route can be perfectly configured, with a valid certificate, and
		// still return 502 to everyone because nothing is listening.
		if answers, checked := route.Answers(); checked && !answers && target.Status == instanceEnums.StatusRunning {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     "nothing-listening",
				Subject:  route.Domain,
				Message:  fmt.Sprintf("Nothing is listening on %s:%d.", route.Target, route.Port),
				Hint:     "The container is running, but the application inside it is not — visitors get 502.",
			})
		}

		if target.Status != instanceEnums.StatusRunning {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     "target-down",
				Subject:  route.Domain,
				Message:  fmt.Sprintf("Points at %s, which is %s.", target.Name, target.Status),
			})
		}
	}

	for _, i := range is {
		if i.Domain == "" {
			continue
		}
		found := false
		for _, route := range rs {
			if route.Domain == i.Domain {
				found = true
				if route.Target != i.Address {
					findings = append(findings, Finding{
						Severity: SeverityError,
						Kind:     "stale-route",
						Subject:  i.Domain,
						Message: fmt.Sprintf("Routes to %s but %s is at %s.",
							route.Target, i.Name, i.Address),
						Hint: "The container moved. The proxy is sending traffic nowhere.",
					})
				}
			}
		}
		if !found {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Kind:     "missing-route",
				Subject:  i.Name,
				Message:  fmt.Sprintf("Expects the domain %s, but no route serves it.", i.Domain),
			})
		}
	}

	return append(findings, inspectCertificates(cs, rs, now)...)
}

// inspectCertificates is separate because a certificate is not attached to a
// container: it belongs to a domain. Only one a site is served with is worth a
// warning: the files a renewal left behind, or certbot's copy of a name croft
// took over, expire without anybody noticing — and rightly so.
func inspectCertificates(cs []*certificates.Certificate, rs []*routes.Route, now time.Time) []Finding {
	findings := []Finding{}

	for _, certificate := range cs {
		if Unused(certificate, rs) {
			continue
		}
		days := certificate.DaysLeft(now)

		switch {
		case certificate.Expired(now):
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     "certificate-expired",
				Subject:  certificate.Domain,
				Message:  fmt.Sprintf("The certificate expired %d days ago.", -days),
				Hint:     "Browsers are refusing this domain right now.",
			})
		case days < 7:
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     "certificate-expiring",
				Subject:  certificate.Domain,
				Message:  fmt.Sprintf("The certificate expires in %d days.", days),
				Hint:     "Whatever should be renewing it has not.",
			})
		case certificate.NeedsAttention(now):
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Kind:     "certificate-expiring",
				Subject:  certificate.Domain,
				Message:  fmt.Sprintf("The certificate expires in %d days.", days),
			})
		}

		if certificate.SelfSigned {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Kind:     "certificate-self-signed",
				Subject:  certificate.Domain,
				Message:  "This certificate signed itself.",
				Hint:     "Browsers will warn about it. Fine for a private service, not for a public one.",
			})
		}
	}
	return findings
}

// Unused is whether no site on the host is served with this certificate.
//
// A certificate is found by its files, and the files of one nobody serves any
// more stay on disk: certbot's copy of a name croft took over, for instance,
// which certbot stops renewing and croft never deletes. Matched by name, it
// warned about an expiry no visitor would ever see, beside the certificate
// actually served, two months from it.
//
// What a site is served with is read from its ssl_certificate line. Where
// that could not be read, a certificate that covers the name is assumed to
// be the one: a false warning costs a look, a missing one costs the site.
func Unused(certificate *certificates.Certificate, rs []*routes.Route) bool {
	if certificate.Path == "" {
		return false
	}
	dir := path.Dir(certificate.Path)

	type served struct{ name, dir string }
	var all []served
	for _, r := range rs {
		if r.SSL {
			all = append(all, served{r.Domain, r.Certificates})
		}
		for _, a := range r.Aliases {
			if a.SSL {
				all = append(all, served{a.Domain, a.Certificates})
			}
		}
	}

	for _, s := range all {
		if s.dir == dir {
			return false
		}
		if s.dir == "" && covers(certificate, s.name) {
			return false
		}
	}
	return true
}

func covers(certificate *certificates.Certificate, name string) bool {
	for _, n := range append([]string{certificate.Domain}, certificate.Names...) {
		if n == name {
			return true
		}
		if base, ok := strings.CutPrefix(n, "*."); ok {
			label, found := strings.CutSuffix(name, "."+base)
			if found && label != "" && !strings.Contains(label, ".") {
				return true
			}
		}
	}
	return false
}

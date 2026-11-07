package reconcile_test

import (
	"strings"
	"testing"
	"time"

	certificates "github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	instances "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceEnums "github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	routes "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/reconcile"
)

func container(name, address string, status instanceEnums.InstanceStatus) *instances.Instance {
	return instances.NewInstance(instances.InstanceProps{
		Name: name, Address: address, Status: status, Managed: true,
	})
}

func route(domain, target string, state routeEnums.ManagedState) *routes.Route {
	return routes.NewRoute(routes.RouteProps{
		Domain: domain, Target: target, Port: 80, State: state,
	})
}

func kinds(findings []reconcile.Finding) map[string]reconcile.Finding {
	byKind := map[string]reconcile.Finding{}
	for _, f := range findings {
		byKind[f.Kind] = f
	}
	return byKind
}

func TestAHealthyHostReportsNothing(t *testing.T) {
	findings := reconcile.Inspect(
		[]*instances.Instance{container("helpdesk", "10.0.0.1", instanceEnums.StatusRunning)},
		[]*routes.Route{route("app.example.com", "10.0.0.1", routeEnums.StateManaged)},
		nil,
	)

	if len(findings) != 0 {
		t.Fatalf("invented %d problems: %+v", len(findings), findings)
	}
}

// The panel puts itself behind a domain pointing at the loopback. It is not a
// container, and must not be reported as one that went missing.
func TestTheLoopbackIsNotAMissingContainer(t *testing.T) {
	findings := reconcile.Inspect(
		nil,
		[]*routes.Route{route("panel.example.com", "127.0.0.1", routeEnums.StateManaged)},
		nil,
	)

	if _, accused := kinds(findings)["orphan-route"]; accused {
		t.Fatalf("the panel accused itself: %+v", findings)
	}
}

func TestARouteToNowhereIsAProblem(t *testing.T) {
	findings := reconcile.Inspect(
		nil,
		[]*routes.Route{route("app.example.com", "10.0.0.99", routeEnums.StateManaged)},
		nil,
	)

	found, ok := kinds(findings)["orphan-route"]
	if !ok {
		t.Fatalf("a route to nothing was not reported: %+v", findings)
	}
	if !strings.Contains(found.Message, "10.0.0.99") {
		t.Errorf("the message does not say where: %q", found.Message)
	}
}

func TestADomainOnAStoppedContainerIsAProblem(t *testing.T) {
	findings := reconcile.Inspect(
		[]*instances.Instance{container("staging", "10.0.0.2", instanceEnums.StatusStopped)},
		[]*routes.Route{route("staging.example.com", "10.0.0.2", routeEnums.StateManaged)},
		nil,
	)

	if _, ok := kinds(findings)["target-down"]; !ok {
		t.Fatalf("not reported: %+v", findings)
	}
}

// Editing a file by hand is the product working, not an incident. It must
// never be reported at the severity of an outage.
func TestAHandEditedFileIsNeverAnError(t *testing.T) {
	findings := reconcile.Inspect(
		[]*instances.Instance{container("helpdesk", "10.0.0.1", instanceEnums.StatusRunning)},
		[]*routes.Route{route("app.example.com", "10.0.0.1", routeEnums.StateAdopted)},
		nil,
	)

	found, ok := kinds(findings)["edited-by-hand"]
	if !ok {
		t.Fatal("the edit was not mentioned at all")
	}
	if found.Severity != reconcile.SeverityInfo {
		t.Errorf("reported at severity %q", found.Severity)
	}
}

// A certificate a site is served with is warned about before it expires,
// louder as the day comes.
func TestCertificatesAreReportedBeforeTheyExpire(t *testing.T) {
	now := time.Now()

	cases := map[string]struct {
		days     int
		wantKind string
		want     reconcile.Severity
	}{
		"comfortable": {60, "", ""},
		"soon":        {15, "certificate-expiring", reconcile.SeverityWarning},
		"imminent":    {3, "certificate-expiring", reconcile.SeverityError},
		"expired":     {-2, "certificate-expired", reconcile.SeverityError},
	}

	for label, test := range cases {
		t.Run(label, func(t *testing.T) {
			findings := reconcile.Inspect(nil, []*routes.Route{served("app.example.com", "/var/lib/croft/certificates/app.example.com")}, []*certificates.Certificate{
				certificates.NewCertificate(certificates.CertificateProps{
					Domain:   "app.example.com",
					NotAfter: now.AddDate(0, 0, test.days),
					Path:     "/var/lib/croft/certificates/app.example.com/fullchain.pem",
					Managed:  true,
				}),
			})

			found, ok := kinds(findings)[test.wantKind]
			if test.wantKind == "" {
				if len(findings) != 0 {
					t.Fatalf("warned too early: %+v", findings)
				}
				return
			}
			if !ok {
				t.Fatalf("not reported: %+v", findings)
			}
			if found.Severity != test.want {
				t.Errorf("severity %q, wanted %q", found.Severity, test.want)
			}
		})
	}
}

// served is a site on https, read from a certificate directory. It proxies
// to the loopback, so no container has to exist for it.
func served(domain, dir string) *routes.Route {
	r := route(domain, "127.0.0.1", routeEnums.StateManaged)
	r.SSL = true
	r.Certificates = dir
	return r
}

func certificateAt(domain, dir string, days int, names ...string) *certificates.Certificate {
	return certificates.NewCertificate(certificates.CertificateProps{
		Domain: domain, Names: names, NotAfter: time.Now().AddDate(0, 0, days),
		Path: dir + "/fullchain.pem", Managed: true,
	})
}

// What happened on the test host: croft took the name over with its
// wildcard, certbot stopped renewing its own copy, and its files stayed. The
// panel warned that dev's certificate expired in nine days while dev was
// served with one good for two months more.
func TestACertificateNoSiteServesIsNotWarnedAbout(t *testing.T) {
	findings := reconcile.Inspect(nil,
		[]*routes.Route{served("dev.example.com", "/var/lib/croft/certificates/_.example.com")},
		[]*certificates.Certificate{
			certificateAt("dev.example.com", "/etc/letsencrypt/live/dev.example.com", 9, "dev.example.com"),
			certificateAt("*.example.com", "/var/lib/croft/certificates/_.example.com", 52, "*.example.com", "example.com"),
		})

	if len(findings) != 0 {
		t.Errorf("warned about a certificate nothing serves: %+v", findings)
	}
}

// The one a site is served with is still watched, whichever name it carries.
func TestTheCertificateASiteIsServedWithIsWatched(t *testing.T) {
	findings := reconcile.Inspect(nil,
		[]*routes.Route{served("dev.example.com", "/var/lib/croft/certificates/_.example.com")},
		[]*certificates.Certificate{
			certificateAt("*.example.com", "/var/lib/croft/certificates/_.example.com", 5, "*.example.com"),
		})

	if f, ok := kinds(findings)["certificate-expiring"]; !ok || f.Subject != "*.example.com" {
		t.Errorf("the wildcard dev is served with expires in 5 days, unsaid: %+v", findings)
	}
}

// A vhost laid out some other way says nothing of where its certificate is.
// Then the one that covers its name is taken for it: staying quiet about the
// certificate a site actually uses is the failure that matters.
func TestWhenTheFileSaysNothingTheCertificateForTheNameIsWatched(t *testing.T) {
	findings := reconcile.Inspect(nil,
		[]*routes.Route{served("shop.example.com", "")},
		[]*certificates.Certificate{
			certificateAt("shop.example.com", "/etc/ssl/shop", 10, "shop.example.com"),
		})

	if _, ok := kinds(findings)["certificate-expiring"]; !ok {
		t.Errorf("a site whose certificate could not be read lost its warning: %+v", findings)
	}
}

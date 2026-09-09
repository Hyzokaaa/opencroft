// Package reconcile compares what the system reports with what the resources
// say they want. The difference is drift — and drift is information shown to
// the user, never a change applied on their behalf.
package reconcile

import (
	"fmt"

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
func Inspect(is []*instances.Instance, rs []*routes.Route) []Finding {
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
				Hint:     "Adopt it to manage it from here, or leave it alone.",
			})
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

	return findings
}

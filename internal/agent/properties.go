package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A service's properties can be saved without deploying it. Configuring and
// deploying are two intentions: somebody fixing the start command at night
// may want it to take effect at the next deployment, not now.
//
// What makes that safe to offer is knowing it happened. Every deployment
// leaves a fingerprint of the properties it ran with; the listing compares it
// with what is saved and says when they differ, so a saved change is never
// mistaken for one that is running.

// configurable are the properties a person sets. The commit is what a
// deployment found, and adoption is read off the container; neither is saved
// from a form, and neither counts towards "changed since deployed".
var configurable = []string{
	"repo", "branch", "path", "runtime", "start", "port",
	"install", "build", "packages", "health", "health-contains", "health-status",
}

// Fingerprint is what a service's saved properties come to. It is read from
// the configuration both when it is written and when it is compared, so the
// two can only differ when the properties do.
func Fingerprint(config map[string]string, service string) string {
	keys := append([]string(nil), configurable...)
	sort.Strings(keys)
	sum := sha256.New()
	for _, key := range keys {
		sum.Write([]byte(key + "=" + config[full(service, key)] + "\n"))
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// pending is true when what is saved is not what was deployed. A service
// deployed before fingerprints were kept has none, and is not called changed
// for that alone.
func pending(config map[string]string, service string) bool {
	deployed := config[full(service, "deployed")]
	return deployed != "" && deployed != Fingerprint(config, service)
}

// markDeployed records that what is saved now is what is running.
func (s *Server) markDeployed(r *http.Request, container, service string) error {
	config, err := s.instances.Annotations(r.Context(), container)
	if err != nil {
		return err
	}
	return s.instances.Annotate(r.Context(), container, annotation(service, "deployed"), Fingerprint(config, service))
}

// configurePlan is one config command per property that changes, and nothing
// for the ones that do not.
func (s *Server) configurePlan(r *http.Request) (string, plan.Plan, error) {
	// Only a service that is already here has properties to change: the
	// first deployment is what creates them.
	name, _, err := s.stored(r)
	if err != nil {
		return "", plan.Plan{}, err
	}
	_, wanted, err := s.acceptService(r)
	if err != nil {
		return "", plan.Plan{}, err
	}
	config, err := s.instances.Annotations(r.Context(), name)
	if err != nil {
		return "", plan.Plan{}, err
	}

	record := wanted.Record()
	steps := []plan.Step{}
	// A service deployed before fingerprints were kept has none to compare
	// with. What is saved now is what it runs, so that is recorded first —
	// otherwise the change about to be saved could never show as pending.
	if config[full(wanted.Name, "deployed")] == "" {
		steps = append(steps, plan.Command("Record what "+wanted.Name+" runs now, before changing it",
			s.bin, "config", "set", name, full(wanted.Name, "deployed"), Fingerprint(config, wanted.Name)))
	}
	before := len(steps)
	for _, key := range configurable {
		key := key
		now, value := config[full(wanted.Name, key)], record[key]
		switch {
		case value == now:
		case value == "":
			steps = append(steps, plan.Command("Clear "+key+" for "+wanted.Name,
				s.bin, "config", "unset", name, full(wanted.Name, key)))
		default:
			steps = append(steps, plan.Command("Save "+key+" for "+wanted.Name,
				s.bin, "config", "set", name, full(wanted.Name, key), value))
		}
	}
	if len(steps) == before {
		return "", plan.Plan{}, errors.New("nothing would change")
	}
	return name, plan.New(steps...), nil
}

func (s *Server) planConfigure(w http.ResponseWriter, r *http.Request) {
	_, p, err := s.configurePlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	name, p, err := s.configurePlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	s.streamOn(w, r, name, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return errors.New(strings.TrimSpace(step.Describe + ": " + err.Error()))
			}
		}
		return nil
	})
}

// Package agent is the privileged half of croft.
//
// It runs as root and is the only thing that talks to LXD, Incus and nginx.
// The half that serves HTTP to a browser runs as an ordinary user and reaches
// this over a unix socket.
//
// The agent exposes a closed set of typed operations. There is deliberately no
// "run this command" endpoint: the API describes *what* it wants, and the agent
// decides which commands that means. A compromised API cannot ask for anything
// the agent was not already willing to do.
package agent

import (
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// SocketPath is where the agent listens. Root owns it; the group is what
// grants the API process access.
const SocketPath = "/run/croft/agent.sock"

// InstanceDTO is an instance as it crosses the socket. It is plain data — the
// agent rebuilds its own entity from it and validates every field again.
type InstanceDTO struct {
	Id       string               `json:"id"`
	Name     string               `json:"name"`
	Image    string               `json:"image"`
	Address  string               `json:"address"`
	Port     int                  `json:"port"`
	Domain   string               `json:"domain"`
	CPULimit int                  `json:"cpuLimit"`
	MemLimit string               `json:"memLimit"`
	Status   enums.InstanceStatus `json:"status"`
	Created  string               `json:"created"`
	Managed  bool                 `json:"managed"`
}

type RouteDTO struct {
	Domain string `json:"domain"`
	Target string `json:"target"`
	Port   int    `json:"port"`
	SSL    bool   `json:"ssl"`
	State  string `json:"state"`
	File   string `json:"file"`
	// Answers is false when nothing accepts a connection at Target:Port.
	Answers bool `json:"answers"`
}

type PlanResponse struct {
	Plan plan.Plan `json:"plan"`
}

type AddressResponse struct {
	Address string `json:"address"`
}

type RuntimeResponse struct {
	Flavor       string `json:"flavor"`
	DefaultImage string `json:"defaultImage"`
	Version      string `json:"version"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type CertificateDTO struct {
	Domain     string   `json:"domain"`
	Names      []string `json:"names"`
	Issuer     string   `json:"issuer"`
	NotAfter   string   `json:"notAfter"`
	Path       string   `json:"path"`
	Managed    bool     `json:"managed"`
	SelfSigned bool     `json:"selfSigned"`
}

// DNSCredentialsDTO travels in one direction only. The response carries which
// provider is configured and where it was read from — never the values.
type DNSCredentialsDTO struct {
	Provider   string            `json:"provider"`
	Source     string            `json:"source,omitempty"`
	Configured bool              `json:"configured"`
	Keys       []string          `json:"keys,omitempty"`
	Values     map[string]string `json:"values,omitempty"`
}

// AppDTO is a deployment as it is stored: on the container, as annotations.
// Reading it back means asking the machine, not a cache of the machine.
type AppDTO struct {
	Deployed bool     `json:"deployed"`
	Repo     string   `json:"repo"`
	Branch   string   `json:"branch"`
	Path     string   `json:"path"`
	Runtime  string   `json:"runtime"`
	Install  []string `json:"install"`
	Build    []string `json:"build"`
	Start    string   `json:"start"`
	Port     int      `json:"port"`
	Packages []string `json:"packages"`
}

type LogsResponse struct {
	Lines string `json:"lines"`
}

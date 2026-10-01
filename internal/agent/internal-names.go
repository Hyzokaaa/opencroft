package agent

import (
	"context"
	"strings"
	"sync"
)

// The bridge the containers sit on runs its own DNS: every container answers
// to <name>.lxd (or .incus) from its neighbours. A name survives what an
// address does not — a container rebuilt, moved, or given another address —
// so it is what a .env should hold when one service reaches another.
//
// It is read once per agent run. The bridge's DNS domain is set when the
// runtime is initialised and next to never changes; asking twice per page
// would be two processes per poll for the same answer.

type internalDNS struct {
	once   sync.Once
	domain string
}

func (s *Server) internalDomain(ctx context.Context) string {
	s.dns.once.Do(func() {
		s.dns.domain = s.readInternalDomain(ctx)
	})
	return s.dns.domain
}

func (s *Server) readInternalDomain(ctx context.Context) string {
	bridge := "lxdbr0"
	fallback := "lxd"
	if s.bin == "incus" {
		bridge, fallback = "incusbr0", "incus"
	}
	if out, err := s.host.Run(ctx, s.bin, "profile", "device", "get", "default", "eth0", "network"); err == nil {
		if name := strings.TrimSpace(out.Stdout); name != "" {
			bridge = name
		}
	}

	// A bridge with its DNS turned off answers for nobody, and offering a
	// name that does not resolve would be worse than offering none.
	if out, err := s.host.Run(ctx, s.bin, "network", "get", bridge, "dns.mode"); err == nil &&
		strings.TrimSpace(out.Stdout) == "none" {
		return ""
	}
	out, err := s.host.Run(ctx, s.bin, "network", "get", bridge, "dns.domain")
	if err != nil {
		return ""
	}
	if domain := strings.TrimSpace(out.Stdout); domain != "" {
		return domain
	}
	return fallback
}

// internalName is how the other containers reach this one, or empty when the
// bridge gives names to nobody.
func (s *Server) internalName(ctx context.Context, container string) string {
	if domain := s.internalDomain(ctx); domain != "" {
		return container + "." + domain
	}
	return ""
}

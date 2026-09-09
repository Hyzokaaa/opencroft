// Package runtime implements InstanceRepository on top of a container runtime.
// LXD and Incus share a command surface but not their image remotes, so the
// flavour is detected once and carried in the driver.
package runtime

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

type Flavor string

const (
	FlavorLXD   Flavor = "lxd"
	FlavorIncus Flavor = "incus"
	FlavorNone  Flavor = "none"
)

// AnnotationPrefix namespaces the desired state we store on the container
// itself, so it survives losing our own database.
const AnnotationPrefix = "user.croft"

// Static addresses are taken from the top of the bridge subnet, away from
// whatever the runtime's DHCP hands out.
const (
	staticFirst = 200
	staticLast  = 250
)

// Detect finds an installed runtime, preferring Incus.
func Detect(ctx context.Context, h host.Host) (Flavor, string) {
	if path, ok := h.Lookup(ctx, "incus"); ok {
		return FlavorIncus, path
	}
	if path, ok := h.Lookup(ctx, "lxc"); ok {
		return FlavorLXD, path
	}
	if path, ok := h.Lookup(ctx, "/snap/bin/lxc"); ok {
		return FlavorLXD, path
	}
	return FlavorNone, ""
}

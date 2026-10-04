package server

import (
	"net"
	"sort"
	"strings"

	overviewQueries "github.com/Hyzokaaa/opencroft/internal/overview/application/queries"
)

// hostAddresses is every IPv4 this machine answers on from outside itself:
// public ones first, then private, never the loopback or the bridge the
// containers sit on. It is read by this process, not the agent, because it
// needs no privileges — and because the moment it matters most is when
// somebody is looking for where the server went, which may well be when the
// agent is the thing that is down.
func hostAddresses() []overviewQueries.HostAddress {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	found := []overviewQueries.HostAddress{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || bridge(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			found = append(found, overviewQueries.HostAddress{Address: ip.String(), Public: !ip.IsPrivate()})
		}
	}

	sort.SliceStable(found, func(a, b int) bool { return found[a].Public && !found[b].Public })
	return found
}

// bridge is an interface the containers live behind rather than one the
// world reaches this machine on.
func bridge(name string) bool {
	for _, prefix := range []string{"lxdbr", "incusbr", "docker", "veth", "br-", "virbr"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

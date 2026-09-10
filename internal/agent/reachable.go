package agent

import (
	"net"
	"strconv"
	"time"
)

// answers reports whether anything accepts a connection at the address a route
// points to.
//
// A route can be perfectly configured, with a valid certificate, and still
// return 502 to everyone because the container has nothing listening. The
// panel used to say "all good" in that situation, which is the panel being
// wrong about the world — the one thing it must not be.
func answers(address string, port int) bool {
	if address == "" || port == 0 {
		return false
	}

	// Short: this runs for every route on every refresh, and a container on
	// the same host answers in microseconds or not at all.
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort(address, strconv.Itoa(port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

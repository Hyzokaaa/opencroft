package agent

import (
	"net"
	"os"
	"strings"
)

// NotifyReady tells systemd the agent is ready — called once its socket
// exists. With Type=notify, systemd only then counts the agent as started, and
// the panel ordered after it starts with the socket already there. Under any
// other service manager, or none, there is nobody to tell and it does nothing.
func NotifyReady() error {
	target := os.Getenv("NOTIFY_SOCKET")
	if target == "" {
		return nil
	}
	// An abstract socket is spelled with a leading @ and addressed with NUL.
	if strings.HasPrefix(target, "@") {
		target = "\x00" + target[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: target, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte("READY=1"))
	return err
}

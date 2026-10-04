package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrUnreachable is the agent not answering at all: no socket, or nothing
// listening on it. It is told apart from every other failure because it is
// not about what was asked — nothing can be read or changed until the agent
// is back, and the panel says exactly that instead of whatever error the
// attempt happened to produce.
var ErrUnreachable = errors.New("the agent is not answering")

// unreachable wraps a failure to reach the socket, and leaves any other error
// as it was.
func unreachable(err error) error {
	var dial *net.OpError
	if errors.As(err, &dial) {
		return fmt.Errorf("%w (%v)", ErrUnreachable, err)
	}
	return err
}

// Connect returns a client for the agent on socket without needing it to be
// there yet. Every call tries the socket again, so a panel started before its
// agent — or one whose agent restarted — recovers on its own the moment the
// agent answers, with nothing to restart.
//
// The handshake that Dial does at once happens here on the first call that
// reaches the agent: it learns the runtime, and refuses to go on if the agent
// is a different version from this half.
func Connect(socket, version string) *Client {
	c := newClient(socket)
	c.version = version
	c.lazy = true
	return c
}

func newClient(socket string) *Client {
	c := &Client{
		http: &http.Client{
			Timeout: 10 * time.Minute, // creating a container is not quick
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					d := net.Dialer{Timeout: 3 * time.Second}
					return d.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
	c.socket = socket
	return c
}

// handshake learns what the agent runs, once it answers. It is retried until
// it succeeds; a version mismatch is not, because waiting will not fix it.
func (c *Client) handshake(ctx context.Context) error {
	c.once.Lock()
	defer c.once.Unlock()
	if c.greeted {
		return nil
	}

	var runtime RuntimeResponse
	if err := c.raw(ctx, http.MethodGet, "/runtime", nil, &runtime); err != nil {
		return err
	}
	if runtime.Version != "" && c.version != "" && runtime.Version != c.version {
		return fmt.Errorf(
			"the agent is running %s and this half is %s. Both are the same binary, so restart "+
				"the one left behind: sudo systemctl restart croft-agent croft",
			runtime.Version, c.version)
	}
	c.flavor = runtime.Flavor
	c.defaultImage = runtime.DefaultImage
	c.greeted = true
	return nil
}

// Reachable answers whether the agent answers right now, and why not.
// It asks every time: a health check that remembered the last answer would
// say the agent is up long after it went down.
func (c *Client) Reachable(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := c.handshake(ctx); err != nil {
		return err
	}
	return c.raw(ctx, http.MethodGet, "/runtime", nil, nil)
}

// greeting is the state kept for the lazy handshake.
type greeting struct {
	once    sync.Mutex
	greeted bool
	lazy    bool
	version string
	socket  string
}

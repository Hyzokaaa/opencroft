package agent

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
)

// chownToGroup hands the socket to a group without pulling in cgo. Membership
// of that group is what lets the unprivileged API talk to the agent.
func chownToGroup(path, group string) error {
	found, err := user.LookupGroup(group)
	if err != nil {
		return fmt.Errorf("no group %q on this host: %w", group, err)
	}

	gid, err := strconv.Atoi(found.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, 0, gid)
}

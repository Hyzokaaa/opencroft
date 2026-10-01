// Package entities holds what a project is: a name some containers share.
package entities

import (
	"errors"
	"regexp"
	"strings"
)

// Annotation is the key on a container that says which project it is in:
// user.croft.project. Belonging lives on the container, so migrating it
// carries the project along and losing the panel's database loses nothing.
const Annotation = "project"

var (
	ErrNameInvalid        = errors.New("a project name is lowercase letters, digits and dashes, like open-helpdesk")
	ErrDescriptionInvalid = errors.New("a description is one line of at most 200 characters")
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// Declaration is a project somebody created on purpose. It is what lets a
// project exist before any container is in it — and it is a file on the
// host, not a row in the panel's database.
type Declaration struct {
	Name        string
	Description string
}

func (d Declaration) Validate() error {
	if !namePattern.MatchString(d.Name) {
		return ErrNameInvalid
	}
	if len(d.Description) > 200 || strings.ContainsAny(d.Description, "\r\n") {
		return ErrDescriptionInvalid
	}
	return nil
}

// Project is what the host says about one: whether it was declared, and which
// containers carry its name. Either alone is enough for it to exist.
type Project struct {
	Name        string
	Description string
	// Declared is false for a name found on containers with no declaration
	// behind it — a container migrated from another host, or one labelled by
	// hand. It is shown, and declaring it is one step.
	Declared  bool
	Instances []string
}

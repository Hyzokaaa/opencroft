package host

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Fake records what would have run instead of running it.
//
// This is what makes the domain testable without LXD, nginx or root: the
// commands a driver produces are the thing worth asserting on, and they are
// exactly what a plan promises.
type Fake struct {
	mu sync.Mutex

	// Commands is every Run, joined, in order.
	Commands []string
	// Files is what was written, by path.
	Files map[string]string
	// Responses is consulted by prefix; the first match wins.
	Responses map[string]string
	// Failures makes any command whose joined form contains the key fail.
	Failures map[string]error
	// Missing are names Lookup should not find.
	Missing map[string]bool
	// Dirs answers ListDir.
	Dirs map[string][]string
}

func NewFake() *Fake {
	return &Fake{
		Files:     map[string]string{},
		Responses: map[string]string{},
		Failures:  map[string]error{},
		Missing:   map[string]bool{},
		Dirs:      map[string][]string{},
	}
}

func (f *Fake) Run(_ context.Context, name string, args ...string) (Output, error) {
	joined := strings.TrimSpace(name + " " + strings.Join(args, " "))

	f.mu.Lock()
	f.Commands = append(f.Commands, joined)
	f.mu.Unlock()

	for fragment, err := range f.Failures {
		if strings.Contains(joined, fragment) {
			return Output{}, err
		}
	}
	for prefix, response := range f.Responses {
		if strings.HasPrefix(joined, prefix) {
			return Output{Stdout: response}, nil
		}
	}
	return Output{}, nil
}

func (f *Fake) ReadFile(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	content, ok := f.Files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(content), nil
}

func (f *Fake) WriteFile(_ context.Context, path string, content []byte, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Files[path] = string(content)
	f.Commands = append(f.Commands, "write "+path)
	return nil
}

func (f *Fake) RemoveFile(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.Files, path)
	f.Commands = append(f.Commands, "remove "+path)
	return nil
}

func (f *Fake) ListDir(_ context.Context, path string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.Dirs[path], nil
}

func (f *Fake) Lookup(_ context.Context, name string) (string, bool) {
	if f.Missing[name] {
		return "", false
	}
	return "/usr/bin/" + name, true
}

// Ran reports whether any recorded command contains the fragment.
func (f *Fake) Ran(fragment string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, command := range f.Commands {
		if strings.Contains(command, fragment) {
			return true
		}
	}
	return false
}

func (f *Fake) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return fmt.Sprintf("%d commands:\n  %s", len(f.Commands), strings.Join(f.Commands, "\n  "))
}

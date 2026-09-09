// Package host abstracts every interaction with the operating system.
// No other layer shells out directly; swapping this out is what makes
// multi-host support a later addition rather than a rewrite.
package host

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type Output struct {
	Stdout string
	Stderr string
}

type Host interface {
	Run(ctx context.Context, name string, args ...string) (Output, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, content []byte, mode os.FileMode) error
	RemoveFile(ctx context.Context, path string) error
	ListDir(ctx context.Context, path string) ([]string, error)
	Lookup(ctx context.Context, name string) (string, bool)
}

// Local runs everything on the machine this process lives on.
type Local struct{}

func NewLocal() *Local {
	return &Local{}
}

func (h *Local) Run(ctx context.Context, name string, args ...string) (Output, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := Output{
		Stdout: strings.TrimRight(stdout.String(), "\n"),
		Stderr: strings.TrimRight(stderr.String(), "\n"),
	}
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out.Stderr)
	}
	return out, nil
}

func (h *Local) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (h *Local) WriteFile(_ context.Context, path string, content []byte, mode os.FileMode) error {
	return os.WriteFile(path, content, mode)
}

func (h *Local) RemoveFile(_ context.Context, path string) error {
	return os.Remove(path)
}

func (h *Local) ListDir(_ context.Context, path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func (h *Local) Lookup(_ context.Context, name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}

// RunStep executes one step of a plan: a command, or a file to write.
func RunStep(ctx context.Context, h Host, s plan.Step) error {
	if s.IsFile() {
		return h.WriteFile(ctx, s.File, []byte(s.Content), 0o644)
	}
	if len(s.Argv) == 0 {
		return nil
	}
	_, err := h.Run(ctx, s.Argv[0], s.Argv[1:]...)
	return err
}

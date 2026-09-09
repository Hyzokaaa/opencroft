// Package plan describes what a write is about to do, before it does it.
//
// A plan is not a preview generated alongside the real work — it *is* the
// work. Execution walks the same steps it showed you, so the two cannot drift
// apart. That is what makes "here is the equivalent by hand" a guarantee
// rather than documentation.
package plan

import "strings"

type Step struct {
	// Describe is for a person: "Pin the address so a restart cannot move it".
	Describe string `json:"describe"`
	// Argv is the command, unquoted and unjoined, exactly as it will run.
	Argv []string `json:"argv"`
	// File is set when the step writes a file rather than running a command.
	File    string `json:"file,omitempty"`
	Content string `json:"content,omitempty"`
	// Optional steps are allowed to fail. Overriding an inherited device is
	// one: it errors when the device is already local, which is fine.
	Optional bool `json:"optional,omitempty"`
}

func Command(describe string, argv ...string) Step {
	return Step{Describe: describe, Argv: argv}
}

func Optional(describe string, argv ...string) Step {
	return Step{Describe: describe, Argv: argv, Optional: true}
}

func WriteFile(describe, path, content string) Step {
	return Step{Describe: describe, File: path, Content: content}
}

func (s Step) IsFile() bool {
	return s.File != ""
}

// Shell renders the step the way you would type it.
func (s Step) Shell() string {
	if s.IsFile() {
		return "cat > " + s.File + " <<'EOF'\n" + s.Content + "EOF"
	}
	return strings.Join(s.Argv, " ")
}

type Plan struct {
	Steps []Step `json:"steps"`
}

func New(steps ...Step) Plan {
	return Plan{Steps: steps}
}

func (p Plan) Shell() string {
	lines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		lines = append(lines, s.Shell())
	}
	return strings.Join(lines, "\n")
}

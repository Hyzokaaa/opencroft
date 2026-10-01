// Package files keeps project declarations as files on the host, one per
// project, where anybody with a shell can read them and a backup of /etc
// takes them along.
package files

import (
	"bufio"
	"context"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/project/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

const Dir = "/etc/croft/projects"

type FileProjectRepository struct {
	host host.Host
	// bin is lxc or incus: the label on a container is written with the
	// runtime's own command, like every other annotation.
	bin string
}

func NewFileProjectRepository(h host.Host, bin string) *FileProjectRepository {
	return &FileProjectRepository{host: h, bin: bin}
}

func file(name string) string { return Dir + "/" + name + ".conf" }

func (r *FileProjectRepository) FindAll(ctx context.Context) ([]entities.Declaration, error) {
	names, err := r.host.ListDir(ctx, Dir)
	if err != nil {
		return nil, nil // no directory yet is no projects yet
	}

	found := []entities.Declaration{}
	for _, entry := range names {
		name, ok := strings.CutSuffix(entry, ".conf")
		if !ok {
			continue
		}
		declaration := entities.Declaration{Name: name}
		if declaration.Validate() != nil {
			continue // not a file croft wrote, and not a name it could show
		}
		raw, err := r.host.ReadFile(ctx, file(name))
		if err != nil {
			continue
		}
		declaration.Description = read(string(raw))["description"]
		found = append(found, declaration)
	}
	return found, nil
}

// read takes key = value lines, the shape of every file croft keeps in /etc.
func read(content string) map[string]string {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values
}

func (r *FileProjectRepository) DeclarePlan(declaration entities.Declaration) plan.Plan {
	content := "# A project croft shows, whether or not a container is in it yet.\n" +
		"# Which containers are in it is a label on each: user.croft.project.\n" +
		"description = " + declaration.Description + "\n"
	return plan.New(
		plan.Command("Make a place for project declarations", "mkdir", "-p", Dir),
		plan.WriteFile("Declare the project "+declaration.Name, file(declaration.Name), content),
	)
}

func (r *FileProjectRepository) RemovePlan(name string) plan.Plan {
	return plan.New(plan.Command("Forget the project "+name+" — no container is in it", "rm", file(name)))
}

func (r *FileProjectRepository) AssignPlan(instance, project string) plan.Plan {
	key := "user.croft." + entities.Annotation
	if project == "" {
		return plan.New(plan.Command("Take "+instance+" out of its project",
			r.bin, "config", "unset", instance, key))
	}
	return plan.New(plan.Command("Put "+instance+" in "+project,
		r.bin, "config", "set", instance, key, project))
}

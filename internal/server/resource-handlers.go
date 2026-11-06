package server

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/Hyzokaaa/opencroft/internal/agent"
)

// ContainerResourcesDTO is what one container holds, as its own page reads
// it: services and sites, the units and sites found beside them, and its
// databases. A part that could not be read says why instead of reading as
// empty — "none" and "unread" are different answers.
type ContainerResourcesDTO struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Project string `json:"project,omitempty"`
	Address string `json:"address,omitempty"`

	Services       *agent.ServicesResponse  `json:"services,omitempty"`
	ServicesError  string                   `json:"servicesError,omitempty"`
	Databases      *agent.DatabasesResponse `json:"databases,omitempty"`
	DatabasesError string                   `json:"databasesError,omitempty"`
}

// listResources answers a project's page in one request: what each of its
// containers holds. The panel used to ask container by container, and only
// for databases, so what a project ran was a click away inside each one.
//
// ?project=<name> is one project; ?project= with nothing is the containers in
// none; without the parameter, every container.
func (d Deps) listResources(w http.ResponseWriter, r *http.Request) {
	instances, err := d.Instances.FindAll(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	query := r.URL.Query()
	_, scoped := query["project"]
	project := query.Get("project")

	out := []ContainerResourcesDTO{}
	for _, instance := range instances {
		if scoped && instance.Project != project {
			continue
		}
		out = append(out, ContainerResourcesDTO{
			Name: instance.Name, Status: string(instance.Status),
			Project: instance.Project, Address: instance.Address,
		})
	}

	deployer, _ := d.Services.(Deployer)
	provisioner, _ := d.Databases.(Provisioner)

	// Every container at once: a project's page waits for its slowest
	// container, not for the sum of them.
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(c *ContainerResourcesDTO) {
			defer wg.Done()
			d.readResources(r.Context(), c, deployer, provisioner)
		}(&out[i])
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, out)
}

func (d Deps) readResources(ctx context.Context, c *ContainerResourcesDTO, deployer Deployer, provisioner Provisioner) {
	if deployer == nil {
		c.ServicesError = "this host cannot deploy services"
	} else if services, err := deployer.FindAll(ctx, c.Name); err != nil {
		c.ServicesError = readable(err)
	} else {
		c.Services = &services
	}

	// Both come from the container's annotations, so a stopped container
	// still says what it holds; only whether each one runs is unknown, and
	// the panel says that from the container's status.
	if provisioner == nil {
		c.DatabasesError = "this host cannot provision databases"
	} else if databases, err := provisioner.FindAll(ctx, c.Name); err != nil {
		c.DatabasesError = readable(err)
	} else {
		c.Databases = &databases
	}
}

// readable is a failure said for the row it belongs to. The agent being down
// is said once, by the banner; here it is only why this row is empty.
func readable(err error) string {
	if errors.Is(err, agent.ErrUnreachable) {
		return "the croft agent is not answering"
	}
	return err.Error()
}

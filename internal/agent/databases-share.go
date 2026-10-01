package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"

	databaseEntities "github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	databaseServices "github.com/Hyzokaaa/opencroft/internal/database/domain/services"
	databaseContainer "github.com/Hyzokaaa/opencroft/internal/database/infrastructure/container"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A container connects to a database in another container of its project.
// The data stays beside the application that owns it; the one connecting gets
// a login of its own, let in from its own address only. See Share.

type ShareDTO struct {
	// Location is the container holding the database; Name is the database
	// there, and the name it is known by here.
	Location string `json:"location"`
	Name     string `json:"name"`
}

type OfferDTO struct {
	Container string `json:"container"`
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	DB        string `json:"db"`
}

// instancesByName answers addresses and projects in one listing.
func (s *Server) instancesByName(ctx context.Context) (map[string]*instanceEntities.Instance, error) {
	all, err := s.instances.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*instanceEntities.Instance, len(all))
	for _, i := range all {
		out[i.Name] = i
	}
	return out, nil
}

func (s *Server) side(ctx context.Context, instance *instanceEntities.Instance) (databaseServices.Side, error) {
	config, err := s.instances.Annotations(ctx, instance.Name)
	if err != nil {
		return databaseServices.Side{}, err
	}
	return databaseServices.Side{Name: instance.Name, Project: instance.Project, Config: config}, nil
}

// reach is what a consumer connects to: the provider's name on the bridge when
// it has one, which outlives its address.
func (s *Server) reach(ctx context.Context, provider *instanceEntities.Instance) string {
	if name := s.internalName(ctx, provider.Name); name != "" {
		return name
	}
	return provider.Address
}

func (s *Server) listShareable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	instances, err := s.instancesByName(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	consumer, ok := instances[name]
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no container by that name"))
		return
	}

	out := []OfferDTO{}
	if consumer.Project != "" {
		me, err := s.side(ctx, consumer)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		others := []databaseServices.Side{}
		for _, other := range instances {
			if other.Name == name || other.Project != consumer.Project {
				continue
			}
			if side, err := s.side(ctx, other); err == nil {
				others = append(others, side)
			}
		}
		for _, offer := range databaseServices.Shareable(me, others) {
			out = append(out, OfferDTO{
				Container: offer.Container, Name: offer.Database.Name,
				Engine: string(offer.Database.Engine), DB: offer.Database.DB,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) acceptShare(r *http.Request) (databaseContainer.Share, []string, []string, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	var dto ShareDTO
	if err := decode(r, &dto); err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	if err := validName(dto.Location); err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	if err := databaseServices.ValidName(dto.Name); err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}

	ctx := r.Context()
	instances, err := s.instancesByName(ctx)
	if err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	consumer, provider := instances[name], instances[dto.Location]
	if consumer == nil || provider == nil {
		return databaseContainer.Share{}, nil, nil, errors.New("no container by that name")
	}
	if consumer.Address == "" || provider.Address == "" {
		return databaseContainer.Share{}, nil, nil, errors.New("both containers need an address — is one of them stopped?")
	}

	me, err := s.side(ctx, consumer)
	if err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	them, err := s.side(ctx, provider)
	if err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}
	provided, login, index, err := databaseServices.ShareDatabase(me, them, dto.Name)
	if err != nil {
		return databaseContainer.Share{}, nil, nil, err
	}

	return databaseContainer.Share{
		Bin:      s.bin,
		Provider: provider.Name, ProviderAddress: provider.Address,
		Consumer: consumer.Name, ConsumerAddress: consumer.Address,
		Host:     s.reach(ctx, provider),
		Database: provided, Login: login,
	}, index, stored(me.Config), nil
}

func (s *Server) planShare(w http.ResponseWriter, r *http.Request) {
	share, index, services, err := s.acceptShare(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: share.Plan(index, services)})
}

func (s *Server) share(w http.ResponseWriter, r *http.Request) {
	share, index, services, err := s.acceptShare(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	steps := share.Plan(index, services).Steps

	s.streamOn(w, r, share.Consumer, func(report func(int, string)) error {
		for i, step := range steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				if step.Optional {
					continue
				}
				return explain(step, err)
			}
		}
		return nil
	})
}

// revokePlan lets a consumer go of a database it was connected to. What is
// known of the provider is read from the provider when it is still there;
// when it is gone, the consumer forgets it all the same.
func (s *Server) revokePlan(ctx context.Context, consumer string, connected *databaseEntities.Database,
	removal databaseServices.Removal, services []string) (plan.Plan, string, error) {
	instances, err := s.instancesByName(ctx)
	if err != nil {
		return plan.Plan{}, "", err
	}
	me := instances[consumer]
	if me == nil {
		return plan.Plan{}, "", errors.New("no container by that name")
	}

	held := connected
	providerAddress := ""
	if provider := instances[connected.Location]; provider != nil {
		providerAddress = provider.Address
		if config, err := s.instances.Annotations(ctx, provider.Name); err == nil {
			if found := databaseServices.FromConfig(config, connected.Name); found != nil {
				held = found
			}
		}
	}

	share := databaseContainer.Share{
		Bin:      s.bin,
		Provider: connected.Location, ProviderAddress: providerAddress,
		Consumer: consumer, ConsumerAddress: me.Address,
		Database: held, Login: connected.User,
	}
	warning := "The data stays in " + connected.Location + "; only " + consumer + "'s login to it goes"
	if len(services) > 0 {
		warning += ", and " + strings.Join(services, ", ") + " restart without its credentials"
	}
	return share.Revoke(removal.Keys, removal.Remaining, services), warning + ".", nil
}

// stillConnected refuses to drop a database other containers read from: the
// data would go from under them with no snapshot of theirs to bring it back.
func (s *Server) stillConnected(ctx context.Context, provider, name string) error {
	all, err := s.instances.FindAll(ctx)
	if err != nil {
		return err
	}
	configs := map[string]map[string]string{}
	for _, i := range all {
		if i.Name == provider {
			continue
		}
		if config, err := s.instances.Annotations(ctx, i.Name); err == nil {
			configs[i.Name] = config
		}
	}
	if consumers := databaseServices.Consumers(configs, provider, name); len(consumers) > 0 {
		return errors.New(databaseServices.ErrStillConnected.Error() + ": disconnect " +
			strings.Join(consumers, ", ") + " first")
	}
	return nil
}

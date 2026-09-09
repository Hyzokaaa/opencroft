// Package queries assembles the read model the dashboard needs in one call.
package queries

import (
	"context"

	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/reconcile"
)

type InstanceView struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Domain   string `json:"domain"`
	CPULimit int    `json:"cpuLimit"`
	MemLimit string `json:"memLimit"`
	Status   string `json:"status"`
	Created  string `json:"created"`
	Managed  bool   `json:"managed"`
}

type RouteView struct {
	Domain string `json:"domain"`
	Target string `json:"target"`
	Port   int    `json:"port"`
	SSL    bool   `json:"ssl"`
	State  string `json:"state"`
	File   string `json:"file"`
}

type OverviewResponse struct {
	Runtime   string               `json:"runtime"`
	Demo      bool                 `json:"demo"`
	Instances []InstanceView       `json:"instances"`
	Routes    []RouteView          `json:"routes"`
	Findings  []reconcile.Finding  `json:"findings"`
}

type OverviewQuery struct {
	listInstances *instanceServices.ListInstances
	listRoutes    *routeServices.ListRoutes
	runtime       string
	demo          bool
}

func NewOverviewQuery(
	listInstances *instanceServices.ListInstances,
	listRoutes *routeServices.ListRoutes,
	runtime string,
	demo bool,
) *OverviewQuery {
	return &OverviewQuery{listInstances: listInstances, listRoutes: listRoutes, runtime: runtime, demo: demo}
}

func (q *OverviewQuery) Execute(ctx context.Context) (OverviewResponse, error) {
	instances, err := q.listInstances.Execute(ctx)
	if err != nil {
		return OverviewResponse{}, err
	}

	routes, err := q.listRoutes.Execute(ctx)
	if err != nil {
		return OverviewResponse{}, err
	}

	response := OverviewResponse{
		Runtime:   q.runtime,
		Demo:      q.demo,
		Instances: make([]InstanceView, 0, len(instances)),
		Routes:    make([]RouteView, 0, len(routes)),
		Findings:  reconcile.Inspect(instances, routes),
	}

	for _, i := range instances {
		response.Instances = append(response.Instances, InstanceView{
			Id: i.GetId(), Name: i.Name, Image: i.Image, Address: i.Address,
			Port: i.Port, Domain: i.Domain, CPULimit: i.CPULimit, MemLimit: i.MemLimit,
			Status: string(i.Status), Created: i.Created, Managed: i.Managed,
		})
	}

	for _, r := range routes {
		response.Routes = append(response.Routes, RouteView{
			Domain: r.Domain, Target: r.Target, Port: r.Port,
			SSL: r.SSL, State: string(r.State), File: r.File,
		})
	}

	return response, nil
}

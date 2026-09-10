// Package queries assembles the read model the dashboard needs in one call.
package queries

import (
	"context"
	"time"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/reconcile"
)

type InstanceView struct {
	Id       string   `json:"id"`
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	Address  string   `json:"address"`
	Port     int      `json:"port"`
	Domain   string   `json:"domain"`
	Domains  []string `json:"domains"`
	CPULimit int      `json:"cpuLimit"`
	MemLimit string   `json:"memLimit"`
	Status   string   `json:"status"`
	Created  string   `json:"created"`
	Managed  bool     `json:"managed"`
}

type CertificateView struct {
	Domain     string   `json:"domain"`
	Names      []string `json:"names"`
	Issuer     string   `json:"issuer"`
	Expires    string   `json:"expires"`
	DaysLeft   int      `json:"daysLeft"`
	Path       string   `json:"path"`
	Managed    bool     `json:"managed"`
	SelfSigned bool     `json:"selfSigned"`
}

type RouteView struct {
	Domain  string `json:"domain"`
	Target  string `json:"target"`
	Port    int    `json:"port"`
	SSL     bool   `json:"ssl"`
	State   string `json:"state"`
	File    string `json:"file"`
	Answers bool   `json:"answers"`
}

type OverviewResponse struct {
	Runtime      string              `json:"runtime"`
	Version      string              `json:"version"`
	Demo         bool                `json:"demo"`
	Instances    []InstanceView      `json:"instances"`
	Routes       []RouteView         `json:"routes"`
	Certificates []CertificateView   `json:"certificates"`
	Findings     []reconcile.Finding `json:"findings"`
}

type OverviewQuery struct {
	listInstances    *instanceServices.ListInstances
	listRoutes       *routeServices.ListRoutes
	listCertificates *certificateServices.ListCertificates
	runtime          string
	version          string
	demo             bool
}

func NewOverviewQuery(
	listInstances *instanceServices.ListInstances,
	listRoutes *routeServices.ListRoutes,
	listCertificates *certificateServices.ListCertificates,
	runtime string,
	version string,
	demo bool,
) *OverviewQuery {
	return &OverviewQuery{
		listInstances:    listInstances,
		listRoutes:       listRoutes,
		listCertificates: listCertificates,
		runtime:          runtime,
		version:          version,
		demo:             demo,
	}
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

	certs, err := q.listCertificates.Execute(ctx)
	if err != nil {
		return OverviewResponse{}, err
	}

	response := OverviewResponse{
		Runtime:      q.runtime,
		Version:      q.version,
		Demo:         q.demo,
		Instances:    make([]InstanceView, 0, len(instances)),
		Routes:       make([]RouteView, 0, len(routes)),
		Certificates: make([]CertificateView, 0, len(certs)),
		Findings:     reconcile.Inspect(instances, routes, certs),
	}

	for _, i := range instances {
		// A container does not have to be annotated to have a domain: whoever
		// set up nginx already said so. Read it from the routes instead.
		served := []string{}
		for _, r := range routes {
			if i.Address != "" && r.Target == i.Address {
				served = append(served, r.Domain)
			}
		}

		primary := i.Domain
		if primary == "" && len(served) > 0 {
			primary = served[0]
		}

		response.Instances = append(response.Instances, InstanceView{
			Id: i.GetId(), Name: i.Name, Image: i.Image, Address: i.Address,
			Port: i.Port, Domain: primary, Domains: served,
			CPULimit: i.CPULimit, MemLimit: i.MemLimit,
			Status: string(i.Status), Created: i.Created, Managed: i.Managed,
		})
	}

	for _, r := range routes {
		response.Routes = append(response.Routes, RouteView{
			Domain: r.Domain, Target: r.Target, Port: r.Port,
			SSL: r.SSL, State: string(r.State), File: r.File,
			Answers: answersOf(r),
		})
	}

	now := time.Now()
	for _, c := range certs {
		response.Certificates = append(response.Certificates, CertificateView{
			Domain: c.Domain, Names: c.Names, Issuer: c.Issuer,
			Expires: c.NotAfter.Format(time.RFC3339), DaysLeft: c.DaysLeft(now),
			Path: c.Path, Managed: c.Managed, SelfSigned: c.SelfSigned,
		})
	}

	return response, nil
}

// answersOf defaults to true when nobody checked, so a host that cannot probe
// does not paint every route as broken.
func answersOf(route *routeEntities.Route) bool {
	answers, checked := route.Answers()
	return !checked || answers
}

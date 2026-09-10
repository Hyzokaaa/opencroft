package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	certificateEntities "github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Client implements the repository interfaces by asking the agent. From the
// domain's point of view nothing changed: it still talks to an interface that
// happens to reach the operating system.
type Client struct {
	http         *http.Client
	flavor       string
	defaultImage string
}

// Dial refuses a version mismatch outright. Two halves of the same binary
// speaking different protocols produces errors like a bare 404, which tells
// nobody anything.
func Dial(socket, version string) (*Client, error) {
	c := &Client{
		http: &http.Client{
			Timeout: 10 * time.Minute, // creating a container is not quick
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
		},
	}

	var runtime RuntimeResponse
	if err := c.call(context.Background(), http.MethodGet, "/runtime", nil, &runtime); err != nil {
		return nil, fmt.Errorf("reaching the agent on %s: %w", socket, err)
	}
	if runtime.Version != "" && runtime.Version != version {
		return nil, fmt.Errorf(
			"the agent is running %s and this half is %s.\n"+
				"        Both are the same binary, so restart the one left behind:\n"+
				"            sudo systemctl restart croft-agent croft",
			runtime.Version, version)
	}

	c.flavor = runtime.Flavor
	c.defaultImage = runtime.DefaultImage
	return c, nil
}

func (c *Client) Flavor() string       { return c.flavor }
func (c *Client) DefaultImage() string { return c.defaultImage }

func (c *Client) call(ctx context.Context, method, path string, body, into any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	// The host part is ignored: the transport always dials the socket.
	req, err := http.NewRequestWithContext(ctx, method, "http://agent"+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		var failure ErrorResponse
		_ = json.NewDecoder(res.Body).Decode(&failure)
		if failure.Error == "" {
			failure.Error = res.Status
		}
		return errors.New(failure.Error)
	}

	if into == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(into)
}

// ── InstanceRepository ────────────────────────────────────────────────────────

func (c *Client) FindAll(ctx context.Context) ([]*instanceEntities.Instance, error) {
	var dtos []InstanceDTO
	if err := c.call(ctx, http.MethodGet, "/instances", nil, &dtos); err != nil {
		return nil, err
	}

	out := make([]*instanceEntities.Instance, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, instanceEntities.NewInstance(instanceEntities.InstanceProps{
			Id: dto.Id, Name: dto.Name, Image: dto.Image, Address: dto.Address,
			Port: dto.Port, Domain: dto.Domain, CPULimit: dto.CPULimit,
			MemLimit: dto.MemLimit, Status: dto.Status, Created: dto.Created,
			Managed: dto.Managed,
		}))
	}
	return out, nil
}

func (c *Client) FindByName(ctx context.Context, name string) (*instanceEntities.Instance, error) {
	all, err := c.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, i := range all {
		if i.Name == name {
			return i, nil
		}
	}
	return nil, nil
}

// CreatePlan asks the agent what it would run. The commands travel from the
// privileged side outwards, as a description — never inwards as instructions.
func (c *Client) CreatePlan(instance *instanceEntities.Instance) plan.Plan {
	var response PlanResponse
	if err := c.call(context.Background(), http.MethodPost, "/instances/plan", toDTO(instance), &response); err != nil {
		return plan.New(plan.Command("The agent could not describe this: " + err.Error()))
	}
	return response.Plan
}

func (c *Client) DeletePlan(name string) plan.Plan {
	var response PlanResponse
	path := "/instances/" + url.PathEscape(name) + "/destroy/plan"
	if err := c.call(context.Background(), http.MethodGet, path, nil, &response); err != nil {
		return plan.New(plan.Command("The agent could not describe this: " + err.Error()))
	}
	return response.Plan
}

func (c *Client) Create(ctx context.Context, instance *instanceEntities.Instance) error {
	return c.call(ctx, http.MethodPost, "/instances", toDTO(instance), nil)
}

func (c *Client) Delete(ctx context.Context, name string) error {
	return c.call(ctx, http.MethodDelete, "/instances/"+url.PathEscape(name), nil, nil)
}

func (c *Client) Start(ctx context.Context, name string) error {
	return c.StartWithProgress(ctx, name, nil)
}

func (c *Client) Stop(ctx context.Context, name string) error {
	return c.StopWithProgress(ctx, name, nil)
}

func (c *Client) Annotate(ctx context.Context, name, key, value string) error {
	return errors.New("annotating from the panel is not implemented yet")
}

func (c *Client) AllocateAddress(ctx context.Context) (string, error) {
	var response AddressResponse
	if err := c.call(ctx, http.MethodGet, "/instances/address", nil, &response); err != nil {
		return "", err
	}
	return response.Address, nil
}

// ── RouteRepository ───────────────────────────────────────────────────────────

func (c *Client) Routes(ctx context.Context) ([]*routeEntities.Route, error) {
	var dtos []RouteDTO
	if err := c.call(ctx, http.MethodGet, "/routes", nil, &dtos); err != nil {
		return nil, err
	}

	out := make([]*routeEntities.Route, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, routeEntities.NewRoute(routeEntities.RouteProps{
			Domain: dto.Domain, Target: dto.Target, Port: dto.Port, SSL: dto.SSL,
			State: routeEnums.ManagedState(dto.State), File: dto.File,
		}))
	}
	return out, nil
}

// RouteClient adapts the same connection to the route repository interface.
// Writing routes is not exposed yet: the panel cannot create domains, so the
// agent has no reason to accept the request.
type RouteClient struct {
	client *Client
}

func (c *Client) Routing() *RouteClient {
	return &RouteClient{client: c}
}

func (r *RouteClient) FindAll(ctx context.Context) ([]*routeEntities.Route, error) {
	return r.client.Routes(ctx)
}

func (r *RouteClient) FindByDomain(ctx context.Context, domain string) (*routeEntities.Route, error) {
	all, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, route := range all {
		if route.Domain == domain {
			return route, nil
		}
	}
	return nil, nil
}

// WritePlan asks the agent what writing this route would do. As with
// containers, the commands come from the privileged side as a description.
func (r *RouteClient) WritePlan(route *routeEntities.Route) plan.Plan {
	var response PlanResponse
	if err := r.client.call(context.Background(), http.MethodPost, "/routes/plan", toRouteDTO(route), &response); err != nil {
		return plan.New(plan.Command("The agent could not describe this: " + err.Error()))
	}
	return response.Plan
}

func (r *RouteClient) RemovePlan(domain string) plan.Plan {
	var response PlanResponse
	path := "/routes/" + url.PathEscape(domain) + "/remove/plan"
	if err := r.client.call(context.Background(), http.MethodGet, path, nil, &response); err != nil {
		return plan.New(plan.Command("The agent could not describe this: " + err.Error()))
	}
	return response.Plan
}

func (r *RouteClient) Write(ctx context.Context, route *routeEntities.Route) error {
	return r.client.call(ctx, http.MethodPost, "/routes", toRouteDTO(route), nil)
}

func (r *RouteClient) Remove(ctx context.Context, domain string) error {
	return r.client.call(ctx, http.MethodDelete, "/routes/"+url.PathEscape(domain), nil, nil)
}

// Reload is part of the write plans; there is nothing separate to ask for.
func (r *RouteClient) Reload(ctx context.Context) error {
	return nil
}

func toRouteDTO(route *routeEntities.Route) RouteDTO {
	return RouteDTO{
		Domain: route.Domain, Target: route.Target, Port: route.Port,
		SSL: route.SSL, State: string(route.State), File: route.File,
	}
}

// CreateWithProgress forwards the agent's narration as it arrives, so the
// panel shows real progress rather than a bar that guesses.
func (c *Client) CreateWithProgress(ctx context.Context, instance *instanceEntities.Instance, report func(int, string)) error {
	return c.streamed(ctx, http.MethodPost, "/instances", toDTO(instance), report)
}

func (c *Client) DeleteWithProgress(ctx context.Context, name string, report func(int, string)) error {
	return c.streamed(ctx, http.MethodDelete, "/instances/"+url.PathEscape(name), nil, report)
}

func (c *Client) streamed(ctx context.Context, method, path string, body any, report func(int, string)) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://agent"+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		var failure ErrorResponse
		_ = json.NewDecoder(res.Body).Decode(&failure)
		if failure.Error == "" {
			failure.Error = res.Status
		}
		return errors.New(failure.Error)
	}
	return readProgress(bufio.NewReader(res.Body), report)
}

// CertificateClient reads the certificates the agent can see. They live in
// root-owned directories, which is exactly why the unprivileged half cannot
// read them itself.
type CertificateClient struct {
	client *Client
}

func (c *Client) Certificates() *CertificateClient {
	return &CertificateClient{client: c}
}

func (r *CertificateClient) FindAll(ctx context.Context) ([]*certificateEntities.Certificate, error) {
	var dtos []CertificateDTO
	if err := r.client.call(ctx, http.MethodGet, "/certificates", nil, &dtos); err != nil {
		return nil, err
	}

	out := make([]*certificateEntities.Certificate, 0, len(dtos))
	for _, dto := range dtos {
		expires, _ := time.Parse(time.RFC3339, dto.NotAfter)
		out = append(out, certificateEntities.NewCertificate(certificateEntities.CertificateProps{
			Domain: dto.Domain, Names: dto.Names, Issuer: dto.Issuer,
			NotAfter: expires, Path: dto.Path, Managed: dto.Managed,
			SelfSigned: dto.SelfSigned,
		}))
	}
	return out, nil
}

// DNS reports which provider is configured, without the credentials.
func (c *Client) DNS(ctx context.Context) (DNSCredentialsDTO, error) {
	var response DNSCredentialsDTO
	err := c.call(ctx, http.MethodGet, "/dns", nil, &response)
	return response, err
}

// SaveDNS hands credentials to the privileged side. They pass through this
// process in memory and are never stored or returned here.
func (c *Client) SaveDNS(ctx context.Context, provider string, values map[string]string) error {
	return c.call(ctx, http.MethodPost, "/dns",
		DNSCredentialsDTO{Provider: provider, Values: values}, nil)
}

// ── Turning on HTTPS ──────────────────────────────────────────────────────────

func (r *RouteClient) TLSPlan(ctx context.Context, domain string) (plan.Plan, error) {
	var response PlanResponse
	path := "/routes/" + url.PathEscape(domain) + "/tls/plan"
	err := r.client.call(ctx, http.MethodGet, path, nil, &response)
	return response.Plan, err
}

func (r *RouteClient) EnableTLS(ctx context.Context, domain string, report func(int, string)) error {
	return r.client.streamed(ctx, http.MethodPost,
		"/routes/"+url.PathEscape(domain)+"/tls", nil, report)
}

func (c *Client) StartPlan(name string) plan.Plan {
	return c.powerPlan(name, "start")
}

func (c *Client) StopPlan(name string) plan.Plan {
	return c.powerPlan(name, "stop")
}

func (c *Client) powerPlan(name, action string) plan.Plan {
	var response PlanResponse
	path := "/instances/" + url.PathEscape(name) + "/" + action + "/plan"
	if err := c.call(context.Background(), http.MethodGet, path, nil, &response); err != nil {
		return plan.New(plan.Command("The agent could not describe this: " + err.Error()))
	}
	return response.Plan
}

// StartWithProgress and StopWithProgress exist for the same reason as the
// create pair: the panel shows what the privileged side is doing, as it does it.
func (c *Client) StartWithProgress(ctx context.Context, name string, report func(int, string)) error {
	return c.streamed(ctx, http.MethodPost, "/instances/"+url.PathEscape(name)+"/start", nil, report)
}

func (c *Client) StopWithProgress(ctx context.Context, name string, report func(int, string)) error {
	return c.streamed(ctx, http.MethodPost, "/instances/"+url.PathEscape(name)+"/stop", nil, report)
}

// Expose puts the panel behind a domain of its own.
func (c *Client) Expose(ctx context.Context, domain string, port int, report func(int, string)) error {
	return c.streamed(ctx, http.MethodPost, "/expose",
		exposeRequest{Domain: domain, Port: port}, report)
}

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	config2 "rolld/internal/config"
	"rolld/internal/model"
	"rolld/internal/utils"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
)

var ctx = context.Background()

// nameLayout is the timestamp suffix of container names: <srv>-<nameLayout>
const nameLayout = "01.02-15.04.05"

var (
	// healthClient bounds every health check request, so a container that accepts
	// connections but never answers can't hold the deployment past its deadline.
	healthClient = &http.Client{Timeout: 3 * time.Second}
	adminClient  = &http.Client{Timeout: 10 * time.Second}
)

type Instance struct {
	v *viper.Viper
	c *client.Client
}

func Init() (*Instance, error) {
	cli, err := client.NewClientWithOpts()
	if err != nil {
		return nil, err
	}
	instance := &Instance{c: cli}
	if err := instance.Reload(); err != nil {
		return nil, err
	}
	return instance, nil
}

// Reload re-reads nodes.yaml and compose.yaml, so edits apply to the next command
// without restarting rolld.
func (i *Instance) Reload() error {
	if err := model.Load(); err != nil {
		return err
	}
	v, err := config2.LoadComposeConfig()
	if err != nil {
		return err
	}
	i.v = v
	return nil
}

func findServer(srv string) (model.Server, bool) {
	for _, service := range model.C.UpstreamServer {
		if service.Srv == srv {
			return service, true
		}
	}
	return model.Server{}, false
}

func (i *Instance) composeFor(srv string) (*config2.ComposeConfig, error) {
	sub := i.v.Sub(strings.Join([]string{"services", srv}, "."))
	if sub == nil {
		return nil, fmt.Errorf("srv %s not found in compose.yaml", srv)
	}
	compose := config2.NewComposeConfig()
	compose.Load(sub)
	return compose, nil
}

// serviceNamePattern matches the names StartUp gives containers of srv.
func serviceNamePattern(srv string) *regexp.Regexp {
	return regexp.MustCompile(`^/?` + regexp.QuoteMeta(srv) + `-\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}$`)
}

// LoadServiceContainers returns the running containers of srv, newest first.
//
// Containers are matched by name instead of image: after a moving tag such as
// :main is pulled again, older containers report the image ID instead of the tag.
func (i *Instance) LoadServiceContainers(srv string) ([]types.ContainerJSON, error) {
	filter := filters.NewArgs()
	filter.Add("status", "running")
	list, err := i.c.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: filter,
	})
	if err != nil {
		return nil, err
	}

	pattern := serviceNamePattern(srv)
	var containers []types.ContainerJSON
	for _, ctr := range list {
		if !slices.ContainsFunc(ctr.Names, pattern.MatchString) {
			continue
		}
		inspect, err := i.c.ContainerInspect(ctx, ctr.ID)
		if err != nil {
			return nil, err
		}
		containers = append(containers, inspect)
	}

	slices.SortFunc(containers, func(a, b types.ContainerJSON) int {
		ats, _ := time.Parse(time.RFC3339Nano, a.Created)
		bts, _ := time.Parse(time.RFC3339Nano, b.Created)
		return bts.Compare(ats)
	})

	return containers, nil
}

// hostPort returns the host port the service port of ctr is published on.
func hostPort(ctr types.ContainerJSON) (string, error) {
	if ctr.ContainerJSONBase == nil || ctr.HostConfig == nil {
		return "", fmt.Errorf("container has no host config")
	}
	bindings := ctr.HostConfig.PortBindings
	if len(bindings) != 1 {
		return "", fmt.Errorf("container %s has %d port bindings, expect 1", ctr.Name, len(bindings))
	}
	for _, val := range bindings {
		for _, p := range val {
			return p.HostPort, nil
		}
	}
	return "", fmt.Errorf("container %s has no host port", ctr.Name)
}

func checkHealth(srv model.Server, port string) bool {
	url := fmt.Sprintf("http://localhost:%s%s", port, srv.HealthCheck)
	fmt.Println("Check Health ...", url)
	ticker := time.NewTicker(time.Second * 3)
	defer ticker.Stop()
	timer := time.NewTimer(time.Second * 40)
	defer timer.Stop()
	for {
		select {
		case <-ticker.C:
			if healthy(url) {
				fmt.Println("Health Check Success")
				return true
			}
			log.Println("Health Check Failed")
		case <-timer.C:
			return false
		}
	}
}

func healthy(url string) bool {
	resp, err := healthClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// adminRequest calls the APISIX admin api and returns the response body, failing
// on a non-2xx response.
func adminRequest(method, path string, body io.Reader) ([]byte, error) {
	request, err := http.NewRequest(method, model.C.Admin+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("X-API-KEY", model.C.AdminKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	resp, err := adminClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: admin api response %v: %s", method, path, resp.StatusCode, bytes.TrimSpace(buf))
	}
	return buf, nil
}

func (i *Instance) changeUpstream(srv model.Server, port string) error {
	nodes := map[string]any{
		fmt.Sprintf("docker.local:%v", port): 100,
	}
	buf, _ := json.Marshal(nodes)

	path := fmt.Sprintf("/apisix/admin/upstreams/%v/nodes", srv.ID)
	if _, err := adminRequest(http.MethodPatch, path, bytes.NewReader(buf)); err != nil {
		return fmt.Errorf("change upstream %v: %w", srv.ID, err)
	}
	fmt.Printf("Upstream %v -> docker.local:%v\n", srv.ID, port)
	return nil
}

// upstreamPorts returns the ports the APISIX upstream of srv currently routes to.
func upstreamPorts(srv model.Server) ([]string, error) {
	buf, err := adminRequest(http.MethodGet, fmt.Sprintf("/apisix/admin/upstreams/%v", srv.ID), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Value struct {
			Nodes json.RawMessage `json:"nodes"`
		} `json:"value"`
	}
	if err := json.Unmarshal(buf, &resp); err != nil {
		return nil, fmt.Errorf("parse upstream %v: %w", srv.ID, err)
	}
	ports, err := parseNodePorts(resp.Value.Nodes)
	if err != nil {
		return nil, fmt.Errorf("parse upstream %v nodes: %w", srv.ID, err)
	}
	return ports, nil
}

// parseNodePorts reads the ports of APISIX upstream nodes, given either as
// {"host:port": weight} or as [{"host": ..., "port": ...}].
func parseNodePorts(nodes json.RawMessage) ([]string, error) {
	var hash map[string]any
	if err := json.Unmarshal(nodes, &hash); err == nil {
		ports := make([]string, 0, len(hash))
		for addr := range hash {
			if idx := strings.LastIndex(addr, ":"); idx != -1 {
				ports = append(ports, addr[idx+1:])
			}
		}
		return ports, nil
	}
	var list []struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(nodes, &list); err != nil {
		return nil, err
	}
	ports := make([]string, 0, len(list))
	for _, n := range list {
		ports = append(ports, strconv.Itoa(n.Port))
	}
	return ports, nil
}

func (i *Instance) StartUp(srv string) error {
	service, ok := findServer(srv)
	if !ok {
		return fmt.Errorf("srv %s not found in nodes.yaml", srv)
	}
	compose, err := i.composeFor(srv)
	if err != nil {
		return err
	}
	existing, err := i.LoadServiceContainers(srv)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		fmt.Println("New Service Up")
	}

	cli := i.c
	if err := compose.Pull(cli); err != nil {
		return err
	}
	port := strconv.Itoa(utils.RandomAvailablePort())
	name := service.Srv + "-" + time.Now().Format(nameLayout)

	fmt.Println("Get Random Port", port)

	compose.ApplyPort(strings.Join([]string{port, "tcp"}, "/"), service.Port)
	fmt.Println("Container Create")
	create, err := cli.ContainerCreate(ctx, compose.Config, compose.HostConfig, compose.NetworkingConfig, nil, name)
	if err != nil {
		return err
	}
	// cleanup removes the new container when the deployment fails after creating it.
	cleanup := func(cause error) error {
		if err := cli.ContainerRemove(ctx, create.ID, container.RemoveOptions{Force: true}); err != nil {
			log.Println("Clear Error Container", name, err)
		} else {
			fmt.Println("Clear Error Container", name)
		}
		return cause
	}

	fmt.Println("Container Run")
	if err := cli.ContainerStart(ctx, create.ID, container.StartOptions{}); err != nil {
		return cleanup(err)
	}
	if !checkHealth(service, port) {
		return cleanup(fmt.Errorf("srv %s health check failed", srv))
	}

	fmt.Println("Change Upstream")
	if err := i.changeUpstream(service, port); err != nil {
		// The change may still have been applied (e.g. the response timed out), and
		// removing the container the upstream routes to would take the service down.
		ports, perr := upstreamPorts(service)
		if perr != nil {
			return fmt.Errorf("%w; can't confirm upstream state (%v), keep container %s, check upstream %v before prune", err, perr, name, service.ID)
		}
		if slices.Contains(ports, port) {
			fmt.Println("Upstream already routes to", name)
			return nil
		}
		return cleanup(err)
	}
	return nil
}

func (i *Instance) Prune(srv string) error {
	found := false
	for _, service := range model.C.UpstreamServer {
		if srv != "all" && service.Srv != srv {
			continue
		}
		found = true
		if err := i.pruneService(service); err != nil {
			if srv != "all" {
				return err
			}
			log.Println(err)
		}
	}
	if !found {
		return fmt.Errorf("srv %s not found in nodes.yaml", srv)
	}
	return nil
}

func (i *Instance) pruneService(service model.Server) error {
	containers, err := i.LoadServiceContainers(service.Srv)
	if err != nil {
		return err
	}
	if len(containers) < 2 {
		fmt.Println("Srv ", service.Srv, "only has one container, skip prune")
		return nil
	}
	serving, err := upstreamPorts(service)
	if err != nil {
		return fmt.Errorf("srv %s: can't read upstream %v, skip prune: %w", service.Srv, service.ID, err)
	}
	targets, err := pruneTargets(containers, serving)
	if err != nil {
		return fmt.Errorf("srv %s: %w, skip prune", service.Srv, err)
	}
	if port, err := hostPort(containers[0]); err != nil || !slices.Contains(serving, port) {
		fmt.Println("Warning: upstream", service.ID, "routes to", serving, "not the newest container", containers[0].Name)
	}
	for _, ctr := range targets {
		fmt.Println("Prune Container", ctr.Name)
		if err := i.c.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true}); err != nil {
			log.Println(err)
		}
	}
	return nil
}

// pruneTargets returns the containers prune may remove, given the containers of a
// service (newest first) and the ports its upstream routes to. The newest container
// and every container the upstream routes to are kept, as is any container whose
// port can't be read. It refuses when the upstream routes to none of the containers.
func pruneTargets(containers []types.ContainerJSON, serving []string) ([]types.ContainerJSON, error) {
	found := false
	var targets []types.ContainerJSON
	for idx, ctr := range containers {
		port, err := hostPort(ctr)
		if err == nil && slices.Contains(serving, port) {
			found = true
			continue
		}
		if idx == 0 || err != nil {
			continue
		}
		targets = append(targets, ctr)
	}
	if !found {
		return nil, fmt.Errorf("upstream routes to %v, none of the containers", serving)
	}
	return targets, nil
}

func (i *Instance) Rollback(srv string) error {
	service, ok := findServer(srv)
	if !ok {
		return fmt.Errorf("srv %s not found in nodes.yaml", srv)
	}
	containers, err := i.LoadServiceContainers(srv)
	if err != nil {
		return err
	}
	if len(containers) < 2 {
		return fmt.Errorf("srv %s only has one container, can't rollback", srv)
	}
	port, err := hostPort(containers[1])
	if err != nil {
		return fmt.Errorf("srv %s can't rollback: %w", srv, err)
	}

	if err := i.changeUpstream(service, port); err != nil {
		return err
	}
	if err := i.c.ContainerRemove(ctx, containers[0].ID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("rolled back to %s, but failed to remove %s: %w", containers[1].Name, containers[0].Name, err)
	}
	fmt.Println("Clear Container", containers[0].Name)
	return nil
}

package ctrl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
	"log"
	"net/http"
	"rolld/config"
	"rolld/internal"
	"rolld/utils"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ctx = context.Background()

const api = "http://127.0.0.1:2375"

type ContainerTuple struct {
	Created       time.Time
	ContainerId   string
	ContainerName string
	Containers    []types.ContainerJSON
}

type Instance struct {
	v *viper.Viper
	c *client.Client
}

func Init() *Instance {
	instance := &Instance{}
	v := config.LoadComposeConfig()

	instance.v = v

	cli, err := client.NewClientWithOpts(client.WithHost(api))
	if err != nil {
		log.Println(err)
		return nil
	}
	instance.c = cli
	return instance

}

func (i *Instance) LoadContainerInfo() (map[string]*ContainerTuple, error) {
	filter := filters.NewArgs()
	filter.Add("status", "running")
	list, err := i.c.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: filter,
	})
	if err != nil {
		return nil, err
	}
	m := make(map[string]*ContainerTuple)

	for _, ctr := range list {
		inspect, _ := i.c.ContainerInspect(ctx, ctr.ID)
		ts, _ := time.Parse(time.RFC3339Nano, inspect.Created)
		c, ok := m[ctr.Image]
		if !ok {
			tuple := &ContainerTuple{
				Created:       ts,
				ContainerId:   ctr.ID,
				ContainerName: ctr.Names[0],
				Containers:    []types.ContainerJSON{inspect},
			}
			m[ctr.Image] = tuple
		} else {
			c.Containers = append(c.Containers, inspect)
		}
	}

	for _, v := range m {
		slices.SortFunc(v.Containers, func(a, b types.ContainerJSON) int {
			ats, _ := time.Parse(time.RFC3339Nano, a.Created)
			bts, _ := time.Parse(time.RFC3339Nano, b.Created)
			return bts.Compare(ats)
		})
	}

	return m, nil
}

func checkHealth(srv internal.Server, port string) bool {
	fmt.Println("Check Health ...")
	return true
	ticker := time.NewTicker(time.Second * 3)
	defer ticker.Stop()
	timer := time.NewTimer(time.Second * 60)
	defer timer.Stop()
	for {
		select {
		case <-ticker.C:
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/%s", port, srv.HealthCheck))
			if err == nil && resp.StatusCode == http.StatusOK {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

func (i *Instance) changeUpstream(srv internal.Server, port string) error {
	nodes := map[string]any{
		fmt.Sprintf("docker.local:%v", port): 100,
	}
	buf, _ := json.Marshal(nodes)

	admin := fmt.Sprintf("%v/apisix/admin/upstreams/%v/nodes", internal.C.Admin, srv.ID)
	request, err := http.NewRequest(http.MethodPatch, admin, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	request.Header.Add("X-API-KEY", internal.C.AdminKey)
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	fmt.Printf("Admin Api Response: %v\n", resp.StatusCode)
	return nil
}

func (i *Instance) StartUp(srv string) {
	cli := i.c
	m, err := i.LoadContainerInfo()
	if err != nil {
		return
	}

	for _, service := range internal.C.UpstreamServer {
		if service.Srv == srv {
			compose := config.NewComposeConfig()
			compose.Load(i.v.Sub(strings.Join([]string{"services", service.Srv}, ".")))
			tuple, ok := m[compose.Config.Image]
			if !ok {
				fmt.Println("New Service Up")
			}

			if err := compose.Pull(cli); err != nil {
				log.Fatal(err)
			}
			port := strconv.Itoa(utils.RandomAvailablePort())
			name := service.Srv + "-" + time.Now().Format("01.02-15.04.05")

			fmt.Println("Get Random Port", port)

			compose.ApplyPort(strings.Join([]string{port, "tcp"}, "/"), service.Port)
			fmt.Println("Container Create")
			create, err := cli.ContainerCreate(ctx, compose.Config, compose.HostConfig, compose.NetworkingConfig, nil, name)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Container Run")
			err = cli.ContainerStart(ctx, create.ID, container.StartOptions{})
			if err != nil || !checkHealth(service, port) {
				_ = cli.ContainerRemove(ctx, create.ID, container.RemoveOptions{
					Force: true,
				})
				log.Fatal("Clear Error Container", err)
			}

			fmt.Println("Change Upstream")
			if err = i.changeUpstream(service, port); err != nil {
				log.Fatal(err)
			}

			if ok {
				fmt.Println("Prune Containers")
				for _, val := range tuple.Containers[1:] {
					err = cli.ContainerRemove(ctx, val.ID, container.RemoveOptions{
						Force: true,
					})
					if err != nil {
						log.Println(err)
					}
				}
			}

			return
		}

	}

	fmt.Println("srv", srv, "not found")
}

func (i *Instance) Prune(srv string) {
	info, err := i.LoadContainerInfo()
	if err != nil {
		log.Fatal(err)
		return
	}
	for _, service := range internal.C.UpstreamServer {
		if srv == "all" || service.Srv == srv {
			compose := config.NewComposeConfig()
			compose.Load(i.v.Sub(strings.Join([]string{"services", service.Srv}, ".")))
			tuple, ok := info[compose.Config.Image]

			if !ok || len(tuple.Containers) < 2 {
				log.Println("Srv ", service.Srv, "only has one container, skip prune")
				continue
			}
			for _, ctr := range tuple.Containers[1:] {
				log.Println("Prune Containers")
				err = i.c.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{
					Force: true,
				})
				if err != nil {
					log.Println(err)
				}
			}
			if srv != "all" {
				return
			}
		}
	}
	if srv != "all" {
		fmt.Println("srv", srv, "not found")
	}
}

func (i *Instance) Rollback(srv string) {
	info, err := i.LoadContainerInfo()
	if err != nil {
		log.Fatal(err)
		return
	}
	for _, service := range internal.C.UpstreamServer {
		if service.Srv == srv {
			compose := config.NewComposeConfig()
			compose.Load(i.v.Sub(strings.Join([]string{"services", service.Srv}, ".")))
			tuple, ok := info[compose.Config.Image]

			if !ok || len(tuple.Containers) < 2 {
				log.Println("Srv ", srv, "only has one container, can't rollback")
				return
			}
			bindings := tuple.Containers[1].HostConfig.PortBindings
			if len(bindings) != 1 {
				log.Println("Srv ", srv, "has multi port bindings, can't rollback")
				return
			}
			var port string
		loop:
			for _, val := range bindings {
				for _, p := range val {
					port = p.HostPort
					break loop
				}

			}

			if err := i.changeUpstream(service, port); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	fmt.Println("srv", srv, "not found")
}

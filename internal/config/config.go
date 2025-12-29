package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/spf13/viper"
)

var ctx = context.Background()

type ComposeConfig struct {
	Sub              *viper.Viper
	Config           *container.Config
	HostConfig       *container.HostConfig
	NetworkingConfig *network.NetworkingConfig
}

func (c *ComposeConfig) Load(v *viper.Viper) {
	c.Sub = v
	c.parseConfig()
	c.parseHostConfig()
	c.parseNetworkingConfig()
}

func (c *ComposeConfig) parseConfig() {
	c.Config.Image = c.Sub.GetString("image")
	c.Config.Env = c.Sub.GetStringSlice("environment")
	c.Config.Entrypoint = c.Sub.GetStringSlice("entrypoint")
}

func (c *ComposeConfig) parseHostConfig() {
	c.HostConfig.ExtraHosts = c.Sub.GetStringSlice("extra_hosts")
	policy := c.Sub.GetString("restart")
	if policy == "" {
		policy = string(container.RestartPolicyUnlessStopped)
	}
	c.HostConfig.RestartPolicy = container.RestartPolicy{
		Name: container.RestartPolicyMode(policy),
	}
	c.HostConfig.Binds = c.Sub.GetStringSlice("volumes")

	c.HostConfig.LogConfig = container.LogConfig{
		Type: "local",
		Config: map[string]string{
			"max-size": "50m",
			"max-file": "3",
		},
	}

}

func (c *ComposeConfig) parseNetworkingConfig() {
	networks := make(map[string]*network.EndpointSettings)
	for _, nw := range c.Sub.GetStringSlice("networks") {
		networks[nw] = &network.EndpointSettings{}
	}
	c.NetworkingConfig.EndpointsConfig = networks
}

func NewComposeConfig() *ComposeConfig {
	return &ComposeConfig{
		Config:           &container.Config{},
		HostConfig:       &container.HostConfig{},
		NetworkingConfig: &network.NetworkingConfig{},
	}
}
func (c *ComposeConfig) ApplyPort(export, inner string) {
	c.Config.ExposedPorts = nat.PortSet{
		nat.Port(inner): struct{}{},
	}
	bindings := nat.PortMap{}
	bindings[nat.Port(inner)] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: export,
		},
		{
			HostIP:   "::",
			HostPort: export,
		},
	}
	c.HostConfig.PortBindings = bindings
}

func (c *ComposeConfig) Pull(cli *client.Client) error {
	ref := c.Sub.GetString("image")
	resp, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func(r io.ReadCloser) {
		_ = r.Close()
	}(resp)
	buf := make([]byte, 4096)
	fmt.Printf("Pulling from %s\n", ref)
	for {
		n, err := resp.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		var m map[string]any
		_ = json.Unmarshal(buf[:n], &m)
		r, ok := m["progress"]
		if !ok {
			fmt.Printf("Image Pulled\r")
		} else {
			fmt.Printf("%v\r", r)
		}
	}
	fmt.Println()
	return nil
}

func (c *ComposeConfig) Prune(cli *client.Client) {
	_, err := cli.ImagesPrune(ctx, filters.NewArgs())
	if err != nil {
		return
	}
}

func (c *ComposeConfig) RollStart(cli *client.Client) {

}

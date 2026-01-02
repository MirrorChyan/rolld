package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/cli/cli/config"
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
	log.Println("Pulling image", ref)
	authStr, err := GetRegistryAuth(ref)
	if err != nil {
		log.Printf("Warning: could not get registry auth: %v, trying without auth\n", err)
		authStr = ""
	}

	resp, err := cli.ImagePull(ctx, ref, image.PullOptions{
		RegistryAuth: authStr,
	})
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

func GetRegistryAuth(imageRef string) (string, error) {
	cfg, err := config.Load(config.Dir())
	if err != nil {
		return "", fmt.Errorf("failed to load docker config: %w", err)
	}

	registryHost := extractRegistryHost(imageRef)
	fmt.Printf("Registry host: %s\n", registryHost)

	authConfig, err := cfg.GetAuthConfig(registryHost)
	if err != nil {
		return "", fmt.Errorf("failed to get auth config: %w", err)
	}

	if authConfig.Username == "" && authConfig.IdentityToken == "" {
		return "", fmt.Errorf("no credentials found for registry: %s", registryHost)
	}

	val, err := json.Marshal(authConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth config: %w", err)
	}

	encodedAuth := base64.URLEncoding.EncodeToString(val)
	return encodedAuth, nil
}

func extractRegistryHost(imageRef string) string {
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		imageRef = imageRef[:idx]
	}
	if idx := strings.Index(imageRef, ":"); idx != -1 {
		beforeColon := imageRef[:idx]
		if !strings.Contains(beforeColon, "/") {
			imageRef = beforeColon
		}
	}

	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) == 1 {
		return "https://index.docker.io/v1/"
	}

	firstPart := parts[0]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") || firstPart == "localhost" {
		return firstPart
	}

	return "https://index.docker.io/v1/"
}

package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/moby/term"
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
	fmt.Printf("Pulling from %s\n", ref)
	if err := printPullProgress(resp, os.Stdout); err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	return nil
}

// printPullProgress prints an image pull progress stream to out. The daemon reports
// a failed pull inside the stream rather than as an error of ImagePull, so the
// returned error is the only signal that the pull did not complete.
func printPullProgress(stream io.Reader, out io.Writer) error {
	fd, isTerminal := term.GetFdInfo(out)
	return jsonmessage.DisplayJSONMessagesStream(stream, out, fd, isTerminal, nil)
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

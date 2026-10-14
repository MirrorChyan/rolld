package model

import (
	"fmt"

	"github.com/spf13/viper"
)

var C GatewayConfig

type Server struct {
	ID          string `mapstructure:"id"`
	Srv         string `mapstructure:"srv"`
	HealthCheck string `mapstructure:"health-check"`
	Port        string `mapstructure:"port"`
}

type GatewayConfig struct {
	Admin          string   `mapstructure:"admin"`
	AdminKey       string   `mapstructure:"admin-key"`
	UpstreamServer []Server `mapstructure:"upstream-server"`
}

// Load reads nodes.yaml in the current directory into C.
func Load() error {
	v := viper.New()
	v.SetConfigName("nodes")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("please create a config file named \"nodes.yaml\" in the current directory: %w", err)
	}
	// Decode into a fresh value: decoding over C again on reload would keep stale
	// upstream servers when the list gets shorter.
	var c GatewayConfig
	if err := v.UnmarshalKey("apisix", &c); err != nil {
		return err
	}
	C = c
	return nil
}

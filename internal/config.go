package internal

import (
	"github.com/spf13/viper"
	"log"
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

func init() {
	viper.New()
	viper.SetConfigName("nodes")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		log.Println("Please create a config file named \"nodes.yaml\" in the current directory")
		log.Fatal(err)
	}
	if err = viper.UnmarshalKey("apisix", &C); err != nil {
		log.Fatal(err)
	}
}

package config

import (
	"github.com/spf13/viper"
	"log"
)

func LoadComposeConfig() *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName("compose")
	v.AddConfigPath(".")
	err := v.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}
	return v
}

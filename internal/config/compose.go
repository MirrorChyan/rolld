package config

import (
	"github.com/spf13/viper"
)

// LoadComposeConfig reads compose.yaml in the current directory.
func LoadComposeConfig() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName("compose")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	return v, nil
}

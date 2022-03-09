package main

import (
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

type Config struct {
	*viper.Viper
}

func NewConfig(fs afero.Fs) *Config {
	v := viper.New()

	v.AddConfigPath(".")
	v.SetConfigName("transfer")
	v.SetFs(fs)

	if err := v.ReadInConfig(); err != nil {
		panic(err)
	}

	return &Config{
		Viper: v,
	}
}

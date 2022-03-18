package main

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

type Config struct {
	*viper.Viper
}

func NewConfigFromFile(fs afero.Fs, file string) *Config {
	v := viper.New()

	v.SetFs(fs)
	v.SetConfigFile(file)

	return readConfig(v)
}

func NewConfig(fs afero.Fs) *Config {
	v := viper.New()

	v.AddConfigPath(".")
	v.SetConfigName("transfer")
	v.SetFs(fs)

	return readConfig(v)
}

func readConfig(v *viper.Viper) *Config {
	if err := v.ReadInConfig(); err != nil {
		log.Fatal().
			Err(err).
			Msg("unable to read configuration")
	}

	return &Config{
		Viper: v,
	}
}

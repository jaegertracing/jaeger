package pluginloader

import (
	"os"

	"go.uber.org/zap"
)

const (
	// PluginsDirectoryEnvVar is the name of the env var that defines the location to log
	PluginsDirectoryEnvVar = "PLUGINS_DIRECTORY"
)

// FactoryConfig tells the PluginLoader where to lo sampling type it needs to create.
type FactoryConfig struct {
	PluginsDirectory string
	InitialLogger    *zap.Logger
}

func FactoryConfigFromEnv() FactoryConfig {
	pluginsDirectory := os.Getenv(PluginsDirectoryEnvVar)
	// TODO: A "bootstrap" logger could be provided by main.go instead of creating it here
	logger, _ := zap.NewProduction()
	return FactoryConfig{
		PluginsDirectory: pluginsDirectory,
		InitialLogger:    logger,
	}
}

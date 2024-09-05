// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config is badger's internal configuration data.
type Config struct {
	// TTL holds time-to-live configuration for the badger store.
	TTL TTL `mapstructure:"ttl"`
	// Directories contains the configuration for where items are stored. Ephemeral must be
	// set to false for this configuration to take effect.
	Directories Directories `mapstructure:"directories"`
	// Ephemeral, if set to true, will store data in a temporary file system.
	// If set to true, the configuration in Directories is ignored.
	Ephemeral bool `mapstructure:"ephemeral"`
	// SyncWrites, if set to true, will immediately sync all writes to disk. Note that
	// setting this field to true will affect write performance.
	SyncWrites bool `mapstructure:"consistency"`
	// MaintenanceInterval is the regular interval after which a maintenance job is
	// run on the values in the store.
	MaintenanceInterval time.Duration `mapstructure:"maintenance_interval"`
	// MetricsUpdateInterval is the regular interval after which metrics are collected
	// by Jaeger.
	MetricsUpdateInterval time.Duration `mapstructure:"metrics_update_interval"`
	// ReadOnly opens the data store in read-only mode. Multiple instances can open the same
	// store in read-only mode. Values still in the write-ahead-log must be replayed before opening.
	ReadOnly bool `mapstructure:"read_only"`
}

type TTL struct {
	// SpanStore holds the amount of time that the span store data is stored.
	// Once this duration has passed for a given key, span store data will
	// no longer be accessible.
	Spans time.Duration `mapstructure:"spans"`
}

type Directories struct {
	// Keys contains the directory in which the keys are stored.
	Keys string `mapstructure:"keys"`
	// Values contains the directory in which the values are stored.
	Values string `mapstructure:"values"`
}

const (
	defaultMaintenanceInterval   time.Duration = 5 * time.Minute
	defaultMetricsUpdateInterval time.Duration = 10 * time.Second
	defaultTTL                   time.Duration = time.Hour * 72
)

const (
	prefix                    = "badger"
	suffixKeyDirectory        = ".directory-key"
	suffixValueDirectory      = ".directory-value"
	suffixEphemeral           = ".ephemeral"
	suffixSpanstoreTTL        = ".span-store-ttl"
	suffixSyncWrite           = ".consistency"
	suffixMaintenanceInterval = ".maintenance-interval"
	suffixMetricsInterval     = ".metrics-update-interval" // Intended only for testing purposes
	suffixReadOnly            = ".read-only"
	defaultDataDir            = string(os.PathSeparator) + "data"
	defaultValueDir           = defaultDataDir + string(os.PathSeparator) + "values"
	defaultKeysDir            = defaultDataDir + string(os.PathSeparator) + "keys"
)

func NewConfig() *Config {
	defaultBadgerDataDir := getCurrentExecutableDir()
	return &Config{
		TTL: TTL{
			Spans: defaultTTL,
		},
		SyncWrites: false, // Performance over durability
		Ephemeral:  true,  // Default is ephemeral storage
		Directories: Directories{
			Keys:   defaultBadgerDataDir + defaultKeysDir,
			Values: defaultBadgerDataDir + defaultValueDir,
		},
		MaintenanceInterval:   defaultMaintenanceInterval,
		MetricsUpdateInterval: defaultMetricsUpdateInterval,
	}
}

func getCurrentExecutableDir() string {
	// We ignore the error, this will fail later when trying to start the store
	exec, _ := os.Executable()
	return filepath.Dir(exec)
}

// AddFlags adds flags for Config.
func (c Config) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, c)
}

func addFlags(flagSet *flag.FlagSet, config Config) {
	flagSet.Bool(
		prefix+suffixEphemeral,
		config.Ephemeral,
		"Mark this storage ephemeral, data is stored in tmpfs.",
	)
	flagSet.Duration(
		prefix+suffixSpanstoreTTL,
		config.TTL.Spans,
		"How long to store the data. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.String(
		prefix+suffixKeyDirectory,
		config.Directories.Keys,
		"Path to store the keys (indexes), this directory should reside in SSD disk. Set ephemeral to false if you want to define this setting.",
	)
	flagSet.String(
		prefix+suffixValueDirectory,
		config.Directories.Values,
		"Path to store the values (spans). Set ephemeral to false if you want to define this setting.",
	)
	flagSet.Bool(
		prefix+suffixSyncWrite,
		config.SyncWrites,
		"If all writes should be synced immediately to physical disk. This will impact write performance.",
	)
	flagSet.Duration(
		prefix+suffixMaintenanceInterval,
		config.MaintenanceInterval,
		"How often the maintenance thread for values is ran. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Duration(
		prefix+suffixMetricsInterval,
		config.MetricsUpdateInterval,
		"How often the badger metrics are collected by Jaeger. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Bool(
		prefix+suffixReadOnly,
		config.ReadOnly,
		"Allows to open badger database in read only mode. Multiple instances can open same database in read-only mode. Values still in the write-ahead-log must be replayed before opening.",
	)
}

// InitFromViper initializes Config with properties from viper.
func (c *Config) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	initFromViper(c, v, logger)
}

func initFromViper(config *Config, v *viper.Viper, _ *zap.Logger) {
	config.Ephemeral = v.GetBool(prefix + suffixEphemeral)
	config.Directories.Keys = v.GetString(prefix + suffixKeyDirectory)
	config.Directories.Values = v.GetString(prefix + suffixValueDirectory)
	config.SyncWrites = v.GetBool(prefix + suffixSyncWrite)
	config.TTL.Spans = v.GetDuration(prefix + suffixSpanstoreTTL)
	config.MaintenanceInterval = v.GetDuration(prefix + suffixMaintenanceInterval)
	config.MetricsUpdateInterval = v.GetDuration(prefix + suffixMetricsInterval)
	config.ReadOnly = v.GetBool(prefix + suffixReadOnly)
}

func (c *Config) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

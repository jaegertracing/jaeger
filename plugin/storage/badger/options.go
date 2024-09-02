// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Options store storage plugin related configs
type Options struct {
	Primary NamespaceConfig `mapstructure:",squash"`
	// This storage plugin does not support additional namespaces
}

// NamespaceConfig is badger's internal configuration data.
type NamespaceConfig struct {
	namespace string
	// SpanStoreTTL holds the amount of time that the span store data is stored.
	// Once this duration has passed for a given key, it will no longer be accessible.
	SpanStoreTTL time.Duration `mapstructure:"span_store_ttl"`
	// KeyDirectory is the directory in which the keys are stored. Ephemeral must be
	// set to false for this configuration to take effect.
	KeyDirectory string `mapstructure:"directory_key"`
	// ValueDirectory is the directory in which the values should be stored. Ephemeral must be
	// set to false for this configuration to take effect.
	ValueDirectory string `mapstructure:"directory_value"`
	// Ephemeral, if set to true, will store data in a temporary file system.
	// If set to true, KeyDirectory and ValueDirectory are ignored.
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

const (
	defaultMaintenanceInterval   time.Duration = 5 * time.Minute
	defaultMetricsUpdateInterval time.Duration = 10 * time.Second
	defaultTTL                   time.Duration = time.Hour * 72
)

const (
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

func DefaultNamespaceConfig() NamespaceConfig {
	defaultBadgerDataDir := getCurrentExecutableDir()
	return NamespaceConfig{
		SpanStoreTTL:          defaultTTL,
		SyncWrites:            false, // Performance over durability
		Ephemeral:             true,  // Default is ephemeral storage
		ValueDirectory:        defaultBadgerDataDir + defaultValueDir,
		KeyDirectory:          defaultBadgerDataDir + defaultKeysDir,
		MaintenanceInterval:   defaultMaintenanceInterval,
		MetricsUpdateInterval: defaultMetricsUpdateInterval,
	}
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, _ ...string /* otherNamespaces */) *Options {
	defaultConfig := DefaultNamespaceConfig()
	defaultConfig.namespace = primaryNamespace

	options := &Options{
		Primary: defaultConfig,
	}

	return options
}

func getCurrentExecutableDir() string {
	// We ignore the error, this will fail later when trying to start the store
	exec, _ := os.Executable()
	return filepath.Dir(exec)
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, opt.Primary)
}

func addFlags(flagSet *flag.FlagSet, nsConfig NamespaceConfig) {
	flagSet.Bool(
		nsConfig.namespace+suffixEphemeral,
		nsConfig.Ephemeral,
		"Mark this storage ephemeral, data is stored in tmpfs.",
	)
	flagSet.Duration(
		nsConfig.namespace+suffixSpanstoreTTL,
		nsConfig.SpanStoreTTL,
		"How long to store the data. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.String(
		nsConfig.namespace+suffixKeyDirectory,
		nsConfig.KeyDirectory,
		"Path to store the keys (indexes), this directory should reside in SSD disk. Set ephemeral to false if you want to define this setting.",
	)
	flagSet.String(
		nsConfig.namespace+suffixValueDirectory,
		nsConfig.ValueDirectory,
		"Path to store the values (spans). Set ephemeral to false if you want to define this setting.",
	)
	flagSet.Bool(
		nsConfig.namespace+suffixSyncWrite,
		nsConfig.SyncWrites,
		"If all writes should be synced immediately to physical disk. This will impact write performance.",
	)
	flagSet.Duration(
		nsConfig.namespace+suffixMaintenanceInterval,
		nsConfig.MaintenanceInterval,
		"How often the maintenance thread for values is ran. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Duration(
		nsConfig.namespace+suffixMetricsInterval,
		nsConfig.MetricsUpdateInterval,
		"How often the badger metrics are collected by Jaeger. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Bool(
		nsConfig.namespace+suffixReadOnly,
		nsConfig.ReadOnly,
		"Allows to open badger database in read only mode. Multiple instances can open same database in read-only mode. Values still in the write-ahead-log must be replayed before opening.",
	)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	initFromViper(&opt.Primary, v, logger)
}

func initFromViper(cfg *NamespaceConfig, v *viper.Viper, _ *zap.Logger) {
	cfg.Ephemeral = v.GetBool(cfg.namespace + suffixEphemeral)
	cfg.KeyDirectory = v.GetString(cfg.namespace + suffixKeyDirectory)
	cfg.ValueDirectory = v.GetString(cfg.namespace + suffixValueDirectory)
	cfg.SyncWrites = v.GetBool(cfg.namespace + suffixSyncWrite)
	cfg.SpanStoreTTL = v.GetDuration(cfg.namespace + suffixSpanstoreTTL)
	cfg.MaintenanceInterval = v.GetDuration(cfg.namespace + suffixMaintenanceInterval)
	cfg.MetricsUpdateInterval = v.GetDuration(cfg.namespace + suffixMetricsInterval)
	cfg.ReadOnly = v.GetBool(cfg.namespace + suffixReadOnly)
}

// GetPrimary returns the primary namespace configuration
func (opt *Options) GetPrimary() NamespaceConfig {
	return opt.Primary
}

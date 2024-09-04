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

// NamespaceConfig is badger's internal configuration data
type NamespaceConfig struct {
	SpanStoreTTL   time.Duration `mapstructure:"span_store_ttl"`
	ValueDirectory string        `mapstructure:"directory_value"`
	KeyDirectory   string        `mapstructure:"directory_key"`
	// Setting this to true will ignore ValueDirectory and KeyDirectory
	Ephemeral             bool          `mapstructure:"ephemeral"`
	SyncWrites            bool          `mapstructure:"consistency"`
	MaintenanceInterval   time.Duration `mapstructure:"maintenance_interval"`
	MetricsUpdateInterval time.Duration `mapstructure:"metrics_update_interval"`
	ReadOnly              bool          `mapstructure:"read_only"`
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
// @nocommit func NewOptions(primaryNamespace string, _ ...string /* otherNamespaces */) *Options {
func NewOptions() *Options {
	defaultConfig := DefaultNamespaceConfig()

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
		prefix+suffixEphemeral,
		nsConfig.Ephemeral,
		"Mark this storage ephemeral, data is stored in tmpfs.",
	)
	flagSet.Duration(
		prefix+suffixSpanstoreTTL,
		nsConfig.SpanStoreTTL,
		"How long to store the data. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.String(
		prefix+suffixKeyDirectory,
		nsConfig.KeyDirectory,
		"Path to store the keys (indexes), this directory should reside in SSD disk. Set ephemeral to false if you want to define this setting.",
	)
	flagSet.String(
		prefix+suffixValueDirectory,
		nsConfig.ValueDirectory,
		"Path to store the values (spans). Set ephemeral to false if you want to define this setting.",
	)
	flagSet.Bool(
		prefix+suffixSyncWrite,
		nsConfig.SyncWrites,
		"If all writes should be synced immediately to physical disk. This will impact write performance.",
	)
	flagSet.Duration(
		prefix+suffixMaintenanceInterval,
		nsConfig.MaintenanceInterval,
		"How often the maintenance thread for values is ran. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Duration(
		prefix+suffixMetricsInterval,
		nsConfig.MetricsUpdateInterval,
		"How often the badger metrics are collected by Jaeger. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.Bool(
		prefix+suffixReadOnly,
		nsConfig.ReadOnly,
		"Allows to open badger database in read only mode. Multiple instances can open same database in read-only mode. Values still in the write-ahead-log must be replayed before opening.",
	)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	initFromViper(&opt.Primary, v, logger)
}

func initFromViper(cfg *NamespaceConfig, v *viper.Viper, _ *zap.Logger) {
	cfg.Ephemeral = v.GetBool(prefix + suffixEphemeral)
	cfg.KeyDirectory = v.GetString(prefix + suffixKeyDirectory)
	cfg.ValueDirectory = v.GetString(prefix + suffixValueDirectory)
	cfg.SyncWrites = v.GetBool(prefix + suffixSyncWrite)
	cfg.SpanStoreTTL = v.GetDuration(prefix + suffixSpanstoreTTL)
	cfg.MaintenanceInterval = v.GetDuration(prefix + suffixMaintenanceInterval)
	cfg.MetricsUpdateInterval = v.GetDuration(prefix + suffixMetricsInterval)
	cfg.ReadOnly = v.GetBool(prefix + suffixReadOnly)
}

// GetPrimary returns the primary namespace configuration
func (opt *Options) GetPrimary() NamespaceConfig {
	return opt.Primary
}

// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/asaskevich/govalidator"
)

const (
	defaultMaintenanceInterval   time.Duration = 5 * time.Minute
	defaultMetricsUpdateInterval time.Duration = 10 * time.Second
	defaultTTL                   time.Duration = time.Hour * 72
	defaultDataDir               string        = string(os.PathSeparator) + "data"
	defaultValueDir              string        = defaultDataDir + string(os.PathSeparator) + "values"
	defaultKeysDir               string        = defaultDataDir + string(os.PathSeparator) + "keys"
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

func DefaultConfig() *Config {
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

func (c *Config) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

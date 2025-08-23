// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"errors"
	"time"
)

type Config struct {
	// AggregationInterval defines the length of aggregation window after
	// which the accumulated dependencies are flushed into storage.
	AggregationInterval time.Duration `yaml:"aggregation_interval" valid:"gt=0"`
	// InactivityTimeout specifies the duration of inactivity after which a trace
	// is considered complete and ready for dependency aggregation.
	InactivityTimeout time.Duration `yaml:"inactivity_timeout" valid:"gt=0"`
	// StorageName specifies the storage backend to use for dependency data.
	StorageName string `yaml:"storage_name" valid:"required"`
}

// Validate checks the configuration fields for validity.
func (c *Config) Validate() error {
	if c.AggregationInterval <= 0 {
		return errors.New("aggregation_interval must be greater than 0")
	}
	if c.InactivityTimeout <= 0 {
		return errors.New("inactivity_timeout must be greater than 0")
	}
	if c.StorageName == "" {
		return errors.New("storage_name must be provided and cannot be empty")
	}
	return nil
}

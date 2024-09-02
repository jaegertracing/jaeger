// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import "github.com/asaskevich/govalidator"

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	// MaxTraces is the maximum amount of traces to store in memory.
	// If multi-tenancy is enabled, this limit applies per tenant.
	// Zero value (default) means no limit (Warning: memory usage will be unbounded).
	MaxTraces int `mapstructure:"max_traces"`
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

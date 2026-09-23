// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import "github.com/asaskevich/govalidator"

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	// MaxTraces is the maximum amount of traces to store in memory.
	// Traces are stored in a ring buffer, so MaxTraces must be greater than zero.
	// If multi-tenancy is enabled, this limit applies per tenant.
	MaxTraces int `mapstructure:"max_traces"`
}

func (c *Configuration) Validate() error {
	if c.MaxTraces <= 0 {
		return errInvalidMaxTraces
	}

	_, err := govalidator.ValidateStruct(c)
	return err
}

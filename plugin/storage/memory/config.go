// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import "github.com/asaskevich/govalidator"

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	// MaxTraces is the maximum amount of traces to store to store in memory.
	// If MaxTraces is set to 0 (default), the number of traces stored will be unbounded.
	MaxTraces int `mapstructure:"max_traces"`
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

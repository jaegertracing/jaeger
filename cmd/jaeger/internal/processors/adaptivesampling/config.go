// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

var _ xconfmap.Validator = (*Config)(nil)

type Config struct {
	// all configuration for the processor is in the remotesampling extension
}

func (*Config) Validate() error {
	return nil
}

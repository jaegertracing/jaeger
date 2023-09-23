// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"go.opentelemetry.io/collector/config/confighttp"
)

// Config has the configuration for jaeger-query,
type Config struct {
	confighttp.HTTPServerSettings `mapstructure:",squash"`
}

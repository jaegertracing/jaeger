// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package corscfg

import "go.opentelemetry.io/collector/config/confighttp"

type Options struct {
	AllowedOrigins []string
	AllowedHeaders []string
}

func (o *Options)	ToOTELCorsConfig() *confighttp.CORSConfig{
	return &confighttp.CORSConfig{
		AllowedOrigins: o.AllowedOrigins,
		AllowedHeaders: o.AllowedHeaders,
	}
}
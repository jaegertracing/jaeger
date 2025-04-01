// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package corscfg

import (
	"flag"
	"strings"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/confighttp"
)

const (
	corsPrefix         = ".cors"
	corsAllowedHeaders = corsPrefix + ".allowed-headers"
	corsAllowedOrigins = corsPrefix + ".allowed-origins"
)

type Flags struct {
	Prefix string
}

func (c Flags) AddFlags(flags *flag.FlagSet) {
	flags.String(c.Prefix+corsAllowedHeaders, "", "Comma-separated CORS allowed headers. See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Headers")
	flags.String(c.Prefix+corsAllowedOrigins, "", "Comma-separated CORS allowed origins. See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Origin")
}

func (c Flags) InitFromViper(v *viper.Viper) *confighttp.CORSConfig {
	var p confighttp.CORSConfig

	allowedHeaders := v.GetString(c.Prefix + corsAllowedHeaders)
	allowedOrigins := v.GetString(c.Prefix + corsAllowedOrigins)

	p.AllowedOrigins = strings.Split(strings.ReplaceAll(allowedOrigins, " ", ""), ",")
	p.AllowedHeaders = strings.Split(strings.ReplaceAll(allowedHeaders, " ", ""), ",")

	return &p
}

// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
)

const (
	suffixServerURL      = ".server-url"
	suffixConnectTimeout = ".connect-timeout"

	defaultServerURL      = "http://localhost:9090"
	defaultConnectTimeout = 30 * time.Second
)

type namespaceConfig struct {
	config.Configuration `mapstructure:",squash"`
	namespace            string
}

// Options stores the configuration entries for this storage.
type Options struct {
	Primary namespaceConfig `mapstructure:",squash"`
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string) *Options {
	defaultConfig := config.Configuration{
		ServerURL:      defaultServerURL,
		ConnectTimeout: defaultConnectTimeout,
	}

	return &Options{
		Primary: namespaceConfig{
			Configuration: defaultConfig,
			namespace:     primaryNamespace,
		},
	}
}

// AddFlags from this storage to the CLI.
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	nsConfig := &opt.Primary
	flagSet.String(nsConfig.namespace+suffixServerURL, defaultServerURL, "The Prometheus server's URL, must include the protocol scheme e.g. http://localhost:9090")
	flagSet.Duration(nsConfig.namespace+suffixConnectTimeout, defaultConnectTimeout, "The period to wait for a connection to Prometheus when executing queries.")

	nsConfig.getTLSFlagsConfig().AddFlags(flagSet)
}

// InitFromViper initializes the options struct with values from Viper.
func (opt *Options) InitFromViper(v *viper.Viper) error {
	cfg := &opt.Primary
	cfg.ServerURL = stripWhiteSpace(v.GetString(cfg.namespace + suffixServerURL))
	cfg.ConnectTimeout = v.GetDuration(cfg.namespace + suffixConnectTimeout)
	var err error
	cfg.TLS, err = cfg.getTLSFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to process Prometheus TLS options: %w", err)
	}
	return nil
}

func (config *namespaceConfig) getTLSFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: config.namespace,
	}
}

// stripWhiteSpace removes all whitespace characters from a string.
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}

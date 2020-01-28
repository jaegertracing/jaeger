// Copyright (c) 2019 The Jaeger Authors.
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

package auth

import (
	"github.com/Shopify/sarama"
	"github.com/pkg/errors"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

func setTLSConfiguration(config *tlscfg.Options, saramaConfig *sarama.Config) error {
	if config.Enabled {
		tlsConfig, err := config.Config()
		if err != nil {
			return errors.Wrap(err, "error loading tls config")
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return nil
}

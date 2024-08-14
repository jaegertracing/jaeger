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
	"fmt"

	"github.com/Shopify/sarama"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

func setTLSConfiguration(config *tlscfg.Options, saramaConfig *sarama.Config, logger *zap.Logger) error {
	if config.Enabled {
		tlsConfig, err := config.Config(logger)
		if err != nil {
			return fmt.Errorf("error loading tls config: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return nil
}

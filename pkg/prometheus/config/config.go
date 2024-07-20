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

package config

import (
	"time"

	"github.com/asaskevich/govalidator"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	ServerURL      string        `valid:"required" mapstructure:"endpoint"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	TLS            tlscfg.Options

	TokenFilePath            string `mapstructure:"token_file_path"`
	TokenOverrideFromContext bool   `mapstructure:"token_override_from_context"`

	MetricNamespace   string `mapstructure:"metric_namespace"`
	LatencyUnit       string `mapstructure:"latency_unit"`
	NormalizeCalls    bool   `mapstructure:"normalize_calls"`
	NormalizeDuration bool   `mapstructure:"normalize_duration"`
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

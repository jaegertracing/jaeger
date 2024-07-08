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
	ServerURL                string
	ConnectTimeout           time.Duration
	TLS                      tlscfg.Options
	TokenFilePath            string
	TokenOverrideFromContext bool

	MetricNamespace   string
	LatencyUnit       string
	NormalizeCalls    bool
	NormalizeDuration bool
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
	if c.ServerURL == "" {
		c.ServerURL = source.ServerURL
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = source.ConnectTimeout
	}
	if c.MetricNamespace == "" {
		c.MetricNamespace = source.MetricNamespace
	}
	if c.LatencyUnit == "" {
		c.LatencyUnit = source.LatencyUnit
	}
	if c.NormalizeCalls == false {
		c.NormalizeCalls = source.NormalizeCalls
	}
	if c.NormalizeDuration == false {
		c.NormalizeDuration = source.NormalizeDuration
	}
}

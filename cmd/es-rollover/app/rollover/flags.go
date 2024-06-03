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

package rollover

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
)

const (
	conditions               = "conditions"
	defaultRollbackCondition = "{\"max_age\": \"2d\"}"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	Conditions string
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.String(conditions, defaultRollbackCondition, "conditions used to rollover to a new write index")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Conditions = v.GetString(conditions)
}

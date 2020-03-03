// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	// CollectorTChannel is the default port for TChannel server for sending spans
	CollectorTChannel = 14267
	collectorPort     = "collector.port"
)

// Options holds tchannel collector configuration.
type Options struct {
	CollectorPort int
}

// AddFlags add Options's flags.
func AddFlags(flags *flag.FlagSet) {
	flags.Int(collectorPort, CollectorTChannel, "The TChannel port for the collector service")
}

// InitFromViper initializes Options from viper.
func (cOpts *Options) InitFromViper(v *viper.Viper) *Options {
	cOpts.CollectorPort = v.GetInt(collectorPort)
	return cOpts
}

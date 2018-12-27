// Copyright (c) 2018 The Jaeger Authors.
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

package reporter

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
)

const (
	reporterType = "reporter.type"
	// TCHANNEL is name of tchannel reporter.
	TCHANNEL Type = "tchannel"
	// GRPC is name of gRPC reporter.
	GRPC Type = "grpc"
	// HTTP is name of http reporter.
	HTTP Type = "http"
)

// Type defines type of reporter.
type Type string

// Options holds generic reporter configuration.
type Options struct {
	ReporterType Type
}

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(reporterType, string(TCHANNEL), fmt.Sprintf("Reporter type to use e.g. %s, %s, %s", string(TCHANNEL), string(GRPC), string(HTTP)))
}

// InitFromViper initializes Options with properties retrieved from Viper.
func (b *Options) InitFromViper(v *viper.Viper) *Options {
	b.ReporterType = Type(v.GetString(reporterType))
	return b
}

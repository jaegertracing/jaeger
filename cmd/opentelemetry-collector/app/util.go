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
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector/service/builder"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
)

// GetOTELConfigFile returns name of OTEL config file.
func GetOTELConfigFile() string {
	f := &flag.FlagSet{}
	f.SetOutput(ioutil.Discard)
	builder.Flags(f)
	// parse flags to bind the value
	f.Parse(os.Args[1:])
	return builder.GetConfigFile()
}

// StorageFlags return a function that adds storage flags.
// Storage parameter can contain a comma separated list of supported Jaeger storage backends.
func StorageFlags(storage string) (func(*flag.FlagSet), error) {
	var flagFn []func(*flag.FlagSet)
	for _, s := range strings.Split(storage, ",") {
		switch s {
		case "cassandra":
			flagFn = append(flagFn, cassandra.DefaultOptions().AddFlags)
		case "elasticsearch":
			flagFn = append(flagFn, elasticsearch.DefaultOptions().AddFlags)
		case "kafka":
			flagFn = append(flagFn, kafka.DefaultOptions().AddFlags)
		default:
			return nil, fmt.Errorf("unknown storage type: %s", s)
		}
	}
	return func(flagSet *flag.FlagSet) {
		for _, f := range flagFn {
			f(flagSet)
		}
	}, nil
}

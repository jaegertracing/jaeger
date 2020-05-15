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
	"strings"

	jConfigFile "github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpcplugin"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/processor/resourceprocessor"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/jaegerreceiver"
)

// AddComponentFlags adds all flags exposed by components
func AddComponentFlags(flags *flag.FlagSet) {
	// Jaeger receiver (via sampling strategies receiver) exposes the same flags as exporter.
	jaegerreceiver.AddFlags(flags)
	resourceprocessor.AddFlags(flags)
	jConfigFile.AddConfigFileFlag(flags)
}

// AddStorageFlags return a function that adds storage flags.
// storage parameter can contain a comma separated list of supported Jaeger storage backends.
func AddStorageFlags(storage string) (func(*flag.FlagSet), error) {
	var flagFn []func(*flag.FlagSet)
	for _, s := range strings.Split(storage, ",") {
		switch s {
		case "cassandra":
			flagFn = append(flagFn, cassandra.DefaultOptions().AddFlags)
		case "elasticsearch":
			flagFn = append(flagFn, elasticsearch.DefaultOptions().AddFlags)
		case "kafka":
			flagFn = append(flagFn, kafka.DefaultOptions().AddFlags)
		case "grpc-plugin":
			flagFn = append(flagFn, grpcplugin.DefaultOptions().AddFlags)
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

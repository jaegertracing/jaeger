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

	jFlags "github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/badgerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/cassandraexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/grpcpluginexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/kafkaexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/memoryexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/processor/resourceprocessor"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/jaegerreceiver"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/zipkinreceiver"
	cassandraStorage "github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	esStorage "github.com/jaegertracing/jaeger/plugin/storage/es"
)

// AddComponentFlags adds all flags exposed by components
func AddComponentFlags(flags *flag.FlagSet) {
	// Jaeger receiver (via sampling strategies receiver) exposes the same flags as exporter.
	jaegerreceiver.AddFlags(flags)
	zipkinreceiver.AddFlags(flags)
	resourceprocessor.AddFlags(flags)
	jFlags.AddConfigFileFlag(flags)
}

// AddStorageFlags return a function that adds storage flags.
// storage parameter can contain a comma separated list of supported Jaeger storage backends.
func AddStorageFlags(storage string, enableArchive bool) (func(*flag.FlagSet), error) {
	var flagFn []func(*flag.FlagSet)
	for _, s := range strings.Split(storage, ",") {
		switch s {
		case "memory":
			flagFn = append(flagFn, memoryexporter.AddFlags)
		case "cassandra":
			flagFn = append(flagFn, cassandraexporter.DefaultOptions().AddFlags)
			if enableArchive {
				flagFn = append(flagFn, cassandraStorage.NewOptions("cassandra-archive").AddFlags)
			}
		case "badger":
			flagFn = append(flagFn, badgerexporter.DefaultOptions().AddFlags)
		case "elasticsearch":
			flagFn = append(flagFn, elasticsearchexporter.DefaultOptions().AddFlags)
			if enableArchive {
				flagFn = append(flagFn, esStorage.NewOptions("es-archive").AddFlags)
			}
		case "kafka":
			flagFn = append(flagFn, kafkaexporter.AddFlags)
		case "grpc-plugin":
			flagFn = append(flagFn, grpcpluginexporter.DefaultOptions().AddFlags)
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

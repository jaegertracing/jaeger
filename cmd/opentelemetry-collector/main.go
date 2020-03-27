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

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/open-telemetry/opentelemetry-collector/service"
	"github.com/spf13/viper"

	jflags "github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/defaults"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	jconfig "github.com/jaegertracing/jaeger/pkg/config"
)

func main() {
	handleErr := func(err error) {
		if err != nil {
			log.Fatalf("Failed to run the service: %v", err)
		}
	}

	info := service.ApplicationStartInfo{
		ExeName:  "jaeger-opentelemetry-collector",
		LongName: "Jaeger OpenTelemetry Collector",
		// TODO
		//Version:  version.Version,
		//GitHash:  version.GitHash,
	}

	v := viper.New()
	cmpts := defaults.Components(v)

	svc, err := service.New(service.Parameters{
		ApplicationStartInfo: info,
		Factories:            cmpts,
	})
	handleErr(err)

	// Add Jaeger specific flags to service command
	// this passes flag values to viper.
	cmd := svc.Command()
	jconfig.AddFlags(v,
		cmd,
		jflags.AddConfigFileFlag,
		kafka.DefaultOptions().AddFlags,
		cassandra.DefaultOptions().AddFlags,
		elasticsearch.DefaultOptions().AddFlags,
	)

	// parse flags to propagate Jaeger config file flag value to viper
	cmd.ParseFlags(os.Args)
	err = jflags.TryLoadConfigFile(v)
	if err != nil {
		handleErr(fmt.Errorf("could not load Jaeger configuration file %w", err))
	}

	err = svc.Start()
	handleErr(err)
}

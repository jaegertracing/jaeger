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
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/service"
	"github.com/open-telemetry/opentelemetry-collector/service/builder"
	"github.com/spf13/viper"

	jflags "github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/defaults"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	jconfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage"
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
	storageType := os.Getenv(storage.SpanStorageTypeEnvVar)
	if storageType == "" {
		storageType = "cassandra"
	}

	cmpts := defaults.Components(v)
	var cfgFactory service.ConfigFactory
	if getConfigFile() == "" {
		log.Println("Config file not provided, installing default Jaeger components")
		cfgFactory = func(*viper.Viper, config.Factories) (*configmodels.Config, error) {
			fmt.Println("---> Returning default config")
			return defaults.DefaultConfig(storageType, cmpts), nil
		}
	}

	svc, err := service.New(service.Parameters{
		ApplicationStartInfo: info,
		Factories:            cmpts,
		ConfigFactory:        cfgFactory,
	})
	handleErr(err)

	// Add Jaeger specific flags to service command
	// this passes flag values to viper.
	storageFlags, err := storageFlags(storageType)
	if err != nil {
		handleErr(err)
	}
	cmd := svc.Command()
	jconfig.AddFlags(v,
		cmd,
		jflags.AddConfigFileFlag,
		storageFlags,
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

// getConfigFile returns name of Jaeger config file.
func getConfigFile() string {
	f := &flag.FlagSet{}
	builder.Flags(f)
	// parse flags to get the file
	f.Parse(os.Args)
	return builder.GetConfigFile()
}

// storageFlags return a function that will add storage flags.
// storage parameter can contain a comma separated list of supported Jaeger storage backends.
func storageFlags(storage string) (func(*flag.FlagSet), error) {
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
			fmt.Println("AAA")
			return nil, fmt.Errorf("unknown storage type: %s", s)
		}
	}
	return func(flagSet *flag.FlagSet) {
		for _, f := range flagFn {
			f(flagSet)
		}
	}, nil
}

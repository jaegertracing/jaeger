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

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	googleGRPC "google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	grpcMemory "github.com/jaegertracing/jaeger/plugin/storage/grpc/memory"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

const (
	serviceName = "mem-store"
)

var configPath string

func main() {
	flag.StringVar(&configPath, "config", "", "A path to the plugin's configuration file")
	flag.Parse()

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			panic(err)
		}
	}

	opts := memory.Options{}
	opts.InitFromViper(v)

	tracer, err := jtracer.New(serviceName)
	if err != nil {
		panic(fmt.Errorf("failed to initialize tracer: %w", err))
	}
	defer tracer.Close(context.Background())

	memStorePlugin := grpcMemory.NewStoragePlugin(memory.NewStore(), memory.NewStore())
	service := &shared.PluginServices{
		Store:        memStorePlugin,
		ArchiveStore: memStorePlugin,
	}
	if v.GetBool("enable_streaming_writer") {
		service.StreamingSpanWriter = memStorePlugin
	}
	grpc.ServeWithGRPCServer(service, func(options []googleGRPC.ServerOption) *googleGRPC.Server {
		return plugin.DefaultGRPCServer([]googleGRPC.ServerOption{
			googleGRPC.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracer.OTEL))),
		})
	})
}

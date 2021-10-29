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
	"flag"
	"path"
	"strings"

	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/hashicorp/go-plugin"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	googleGRPC "google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	serviceName = "mem-store"
)

var configPath string

func main() {
	flag.StringVar(&configPath, "config", "", "A path to the plugin's configuration file")
	flag.Parse()

	if configPath != "" {
		viper.SetConfigFile(path.Base(configPath))
		viper.AddConfigPath(path.Dir(configPath))
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	opts := memory.Options{}
	opts.InitFromViper(v)

	traceCfg := &jaegerClientConfig.Configuration{
		ServiceName: serviceName,
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "const",
			Param: 1.0,
		},
		RPCMetrics: true,
	}

	tracer, closer, err := traceCfg.NewTracer()
	if err != nil {
		panic("Failed to initialize tracer")
	}
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	memStorePlugin := &memoryStorePlugin{
		store:        memory.NewStore(),
		archiveStore: memory.NewStore(),
	}
	grpc.ServeWithGRPCServer(&shared.PluginServices{
		Store:        memStorePlugin,
		ArchiveStore: memStorePlugin,
	}, func(options []googleGRPC.ServerOption) *googleGRPC.Server {
		return plugin.DefaultGRPCServer([]googleGRPC.ServerOption{
			googleGRPC.UnaryInterceptor(otgrpc.OpenTracingServerInterceptor(tracer)),
			googleGRPC.StreamInterceptor(otgrpc.OpenTracingStreamServerInterceptor(tracer)),
		})
	})
}

type memoryStorePlugin struct {
	store        *memory.Store
	archiveStore *memory.Store
}

func (ns *memoryStorePlugin) DependencyReader() dependencystore.Reader {
	return ns.store
}

func (ns *memoryStorePlugin) SpanReader() spanstore.Reader {
	return ns.store
}

func (ns *memoryStorePlugin) SpanWriter() spanstore.Writer {
	return ns.store
}

func (ns *memoryStorePlugin) ArchiveSpanReader() spanstore.Reader {
	return ns.archiveStore
}

func (ns *memoryStorePlugin) ArchiveSpanWriter() spanstore.Writer {
	return ns.archiveStore
}

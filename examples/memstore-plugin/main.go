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

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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

	plugin := &memoryStorePlugin{
		store:        memory.NewStore(),
		archiveStore: memory.NewStore(),
	}
	grpc.Serve(&shared.PluginServices{
		Store:        plugin,
		ArchiveStore: plugin,
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

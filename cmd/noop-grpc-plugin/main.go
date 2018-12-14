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
	"github.com/hashicorp/go-plugin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/spf13/viper"
	"path"
	"strings"
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

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		VersionedPlugins: map[int]plugin.PluginSet{
			1: map[string]plugin.Plugin{
				shared.StoragePluginIdentifier: &shared.StorageGRPCPlugin{
					Impl: &noopStore{},
				},
			},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

type noopStore struct {

}

func (*noopStore) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return nil, nil
}

func (*noopStore) GetServices(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (*noopStore) GetOperations(ctx context.Context, service string) ([]string, error) {
	return nil, nil
}

func (*noopStore) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil,nil
}

func (*noopStore) WriteSpan(span *model.Span) error {
	return nil
}

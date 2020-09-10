// Copyright (c) 2019 The Jaeger Authors.
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

package grpc

import (
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
)

// Serve creates a plugin configuration using the implementation of StoragePlugin and then serves it.
func Serve(services *shared.PluginServices) {
	ServeWithGRPCServer(services, plugin.DefaultGRPCServer)
}

// ServeWithGRPCServer creates a plugin configuration using the implementation of StoragePlugin and
// function to create grpcServer, and then serves it.
func ServeWithGRPCServer(services *shared.PluginServices, grpcServer func([]grpc.ServerOption) *grpc.Server,
) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		VersionedPlugins: map[int]plugin.PluginSet{
			1: map[string]plugin.Plugin{
				shared.StoragePluginIdentifier: &shared.StorageGRPCPlugin{
					Impl:        services.Store,
					ArchiveImpl: services.ArchiveStore,
				},
			},
		},
		GRPCServer: grpcServer,
	})
}

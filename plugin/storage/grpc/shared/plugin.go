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

package shared

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
)

// Ensure plugin.GRPCPlugin API match.
var _ plugin.GRPCPlugin = (*StorageGRPCPlugin)(nil)

// StorageGRPCPlugin is the implementation of plugin.GRPCPlugin.
type StorageGRPCPlugin struct {
	plugin.Plugin

	// Concrete implementation, This is only used for plugins that are written in Go.
	Impl        StoragePlugin
	ArchiveImpl ArchiveStoragePlugin
	StreamImpl  StreamingSpanWriterPlugin
}

// RegisterHandlers registers the plugin with the server
func (p *StorageGRPCPlugin) RegisterHandlers(s *grpc.Server) error {
	handler := NewGRPCHandlerWithPlugins(p.Impl, p.ArchiveImpl, p.StreamImpl)
	return handler.Register(s)
}

// GRPCServer implements plugin.GRPCPlugin. It is used by go-plugin to create a grpc plugin server.
func (p *StorageGRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	return p.RegisterHandlers(s)
}

// GRPCClient implements plugin.GRPCPlugin. It is used by go-plugin to create a grpc plugin client.
func (*StorageGRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return NewGRPCClient(conn), nil
}

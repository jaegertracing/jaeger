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

package shared

import (
	"context"
	"github.com/jaegertracing/jaeger/storage/dependencystore"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/proto"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// StoragePluginIdentifier is the identifier that is shared by plugin and host.
const StoragePluginIdentifier = "storage_plugin"

// Handshake is a common handshake that is shared by plugin and host.
var Handshake = plugin.HandshakeConfig{
	MagicCookieKey:   "STORAGE_PLUGIN",
	MagicCookieValue: "jaeger",
}

// PluginMap is the map of plugins we can dispense.
var PluginMap = map[string]plugin.Plugin{
	StoragePluginIdentifier: &StorageGRPCPlugin{},
}

// StoragePlugin is the interface we're exposing as a plugin.
type StoragePlugin interface {
	spanstore.Reader
	spanstore.Writer
	dependencystore.Reader
}

// This is the implementation of plugin.GRPCPlugin so we can serve/consume this.
type StorageGRPCPlugin struct {
	plugin.Plugin
	// Concrete implementation, written in Go. This is only used for plugins
	// that are written in Go.
	Impl StoragePlugin
}

func (p *StorageGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterStoragePluginServer(s, &GRPCServer{Impl: p.Impl})
	return nil
}

func (*StorageGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{client: proto.NewStoragePluginClient(c)}, nil
}

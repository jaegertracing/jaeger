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

package shared

import (
	"github.com/hashicorp/go-plugin"

	"github.com/jaegertracing/jaeger/storage/dependencystore"
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
	SpanReader() spanstore.Reader
	SpanWriter() spanstore.Writer
	DependencyReader() dependencystore.Reader
}

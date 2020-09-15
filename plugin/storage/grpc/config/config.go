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

package config

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	PluginBinary            string `yaml:"binary" mapstructure:"binary"`
	PluginConfigurationFile string `yaml:"configuration-file" mapstructure:"configuration_file"`
	PluginLogLevel          string `yaml:"log-level" mapstructure:"log_level"`
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
}

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
type PluginBuilder interface {
	Build() (*ClientPluginServices, error)
}

// Build instantiates a PluginServices
func (c *Configuration) Build() (*ClientPluginServices, error) {
	// #nosec G204
	cmd := exec.Command(c.PluginBinary, "--config", c.PluginConfigurationFile)

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: shared.Handshake,
		VersionedPlugins: map[int]plugin.PluginSet{
			1: shared.PluginMap,
		},
		Cmd:              cmd,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.LevelFromString(c.PluginLogLevel),
		}),
	})

	runtime.SetFinalizer(client, func(c *plugin.Client) {
		c.Kill()
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("error attempting to connect to plugin rpc client: %w", err)
	}

	raw, err := rpcClient.Dispense(shared.StoragePluginIdentifier)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve storage plugin instance: %w", err)
	}

	// in practice, the type of `raw` is *shared.grpcClient, and type casts below cannot fail
	storagePlugin, ok := raw.(shared.StoragePlugin)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.StoragePlugin for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}
	archiveStoragePlugin, ok := raw.(shared.ArchiveStoragePlugin)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.ArchiveStoragePlugin for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}
	capabilities, ok := raw.(shared.PluginCapabilities)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.PluginCapabilities for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}

	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:        storagePlugin,
			ArchiveStore: archiveStoragePlugin,
		},
		Capabilities: capabilities,
	}, nil
}

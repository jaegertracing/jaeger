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

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
type PluginBuilder interface {
	Build() (shared.StoragePlugin, error)
}

// Build instantiates a StoragePlugin
func (c *Configuration) Build() (shared.StoragePlugin, shared.ArchiveStoragePlugin, shared.PluginCapabilities, error) {
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
		return nil, nil, nil, fmt.Errorf("error attempting to connect to plugin rpc client: %s", err)
	}

	raw, err := rpcClient.Dispense(shared.StoragePluginIdentifier)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to retrieve storage plugin instance: %s", err)
	}

	storagePlugin, ok := raw.(shared.StoragePlugin)
	if !ok {
		return nil, nil, nil, fmt.Errorf("unexpected type for plugin \"%s\"", shared.StoragePluginIdentifier)
	}

	archiveStoragePlugin := raw.(shared.ArchiveStoragePlugin)
	capabilities := raw.(shared.PluginCapabilities)

	return storagePlugin, archiveStoragePlugin, capabilities, nil
}

// PluginBuilder is used to create storage plugins
type PluginBuilder interface {
	Build() (shared.StoragePlugin, shared.ArchiveStoragePlugin, shared.PluginCapabilities, error)
}

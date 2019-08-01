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

package badger

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Options store storage plugin related configs
type Options struct {
	primary *NamespaceConfig
	// This storage plugin does not support additional namespaces
}

// NamespaceConfig is badger's internal configuration data
type NamespaceConfig struct {
	namespace           string
	SpanStoreTTL        time.Duration
	ValueDirectory      string
	KeyDirectory        string
	Ephemeral           bool // Setting this to true will ignore ValueDirectory and KeyDirectory
	SyncWrites          bool
	MaintenanceInterval time.Duration
}

const (
	defaultMaintenanceInterval time.Duration = 5 * time.Second
	defaultTTL                 time.Duration = time.Hour * 72
)

const (
	suffixKeyDirectory        = ".directory-key"
	suffixValueDirectory      = ".directory-value"
	suffixEphemeral           = ".ephemeral"
	suffixSpanstoreTTL        = ".span-store-ttl"
	suffixSyncWrite           = ".consistency"
	suffixMaintenanceInterval = ".maintenance-interval"
	defaultDataDir            = string(os.PathSeparator) + "data"
	defaultValueDir           = defaultDataDir + string(os.PathSeparator) + "values"
	defaultKeysDir            = defaultDataDir + string(os.PathSeparator) + "keys"
)

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {

	defaultBadgerDataDir := getCurrentExecutableDir()

	options := &Options{
		primary: &NamespaceConfig{
			namespace:           primaryNamespace,
			SpanStoreTTL:        defaultTTL,
			SyncWrites:          false, // Performance over durability
			Ephemeral:           true,  // Default is ephemeral storage
			ValueDirectory:      defaultBadgerDataDir + defaultValueDir,
			KeyDirectory:        defaultBadgerDataDir + defaultKeysDir,
			MaintenanceInterval: defaultMaintenanceInterval,
		},
	}

	return options
}

func getCurrentExecutableDir() string {
	// We ignore the error, this will fail later when trying to start the store
	exec, _ := os.Executable()
	return filepath.Dir(exec)
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, opt.primary)
}

func addFlags(flagSet *flag.FlagSet, nsConfig *NamespaceConfig) {
	flagSet.Bool(
		nsConfig.namespace+suffixEphemeral,
		nsConfig.Ephemeral,
		"Mark this storage ephemeral, data is stored in tmpfs.",
	)
	flagSet.Duration(
		nsConfig.namespace+suffixSpanstoreTTL,
		nsConfig.SpanStoreTTL,
		"How long to store the data. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
	flagSet.String(
		nsConfig.namespace+suffixKeyDirectory,
		nsConfig.KeyDirectory,
		"Path to store the keys (indexes), this directory should reside in SSD disk. Set ephemeral to false if you want to define this setting.",
	)
	flagSet.String(
		nsConfig.namespace+suffixValueDirectory,
		nsConfig.ValueDirectory,
		"Path to store the values (spans). Set ephemeral to false if you want to define this setting.",
	)
	flagSet.Bool(
		nsConfig.namespace+suffixSyncWrite,
		nsConfig.SyncWrites,
		"If all writes should be synced immediately. This will greatly reduce write performance.",
	)
	flagSet.Duration(
		nsConfig.namespace+suffixMaintenanceInterval,
		nsConfig.MaintenanceInterval,
		"How often the maintenance thread for values is ran. Format is time.Duration (https://golang.org/pkg/time/#Duration)",
	)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(opt.primary, v)
}

func initFromViper(cfg *NamespaceConfig, v *viper.Viper) {
	cfg.Ephemeral = v.GetBool(cfg.namespace + suffixEphemeral)
	cfg.KeyDirectory = v.GetString(cfg.namespace + suffixKeyDirectory)
	cfg.ValueDirectory = v.GetString(cfg.namespace + suffixValueDirectory)
	cfg.SyncWrites = v.GetBool(cfg.namespace + suffixSyncWrite)
	cfg.SpanStoreTTL = v.GetDuration(cfg.namespace + suffixSpanstoreTTL)
	cfg.MaintenanceInterval = v.GetDuration(cfg.namespace + suffixMaintenanceInterval)
}

// GetPrimary returns the primary namespace configuration
func (opt *Options) GetPrimary() *NamespaceConfig {
	return opt.primary
}

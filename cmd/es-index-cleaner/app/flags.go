// Copyright (c) 2021 The Jaeger Authors.
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

package app

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	indexPrefix        = "index-prefix"
	archive            = "archive"
	rollover           = "rollover"
	timeout            = "timeout"
	indexDateSeparator = "index-date-separator"
	username           = "es.username"
	password           = "es.password"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	IndexPrefix              string
	Archive                  bool
	Rollover                 bool
	MasterNodeTimeoutSeconds int
	IndexDateSeparator       string
	Username                 string
	Password                 string
	TLSEnabled               bool
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.String(indexPrefix, "", "Index prefix")
	flags.Bool(archive, false, "Whether to remove archive indices. It works only for rollover")
	flags.Bool(rollover, false, "Whether to remove indices created by rollover")
	flags.Int(timeout, 120, "Number of seconds to wait for master node response")
	flags.String(indexDateSeparator, "-", "Index date separator")
	flags.String(username, "", "The username required by storage")
	flags.String(password, "", "The password required by storage")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.IndexPrefix = v.GetString(indexPrefix)
	if c.IndexPrefix != "" {
		c.IndexPrefix += "-"
	}

	c.Archive = v.GetBool(archive)
	c.Rollover = v.GetBool(rollover)
	c.MasterNodeTimeoutSeconds = v.GetInt(timeout)
	c.IndexDateSeparator = v.GetString(indexDateSeparator)
	c.Username = v.GetString(username)
	c.Password = v.GetString(password)
}

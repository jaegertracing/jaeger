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
	indexPrefix      = "index-prefix"
	archive          = "archive"
	username         = "es.username"
	password         = "es.password"
	useILM           = "es.use-ilm"
	ilmPolicyName    = "es.ilm-policy-name"
	timeout          = "timeout"
	skipDependencies = "skip-dependencies"
)

// Config holds the global configurations for the es rollover, common to all actions
type Config struct {
	IndexPrefix      string
	Archive          bool
	Username         string
	Password         string
	TLSEnabled       bool
	ILMPolicyName    string
	UseILM           bool
	Timeout          int
	SkipDependencies bool
}

// AddFlags adds flags
func AddFlags(flags *flag.FlagSet) {
	flags.String(indexPrefix, "", "Index prefix")
	flags.Bool(archive, false, "Handle archive indices")
	flags.String(username, "", "The username required by storage")
	flags.String(password, "", "The password required by storage")
	flags.Bool(useILM, false, "Use ILM to manage jaeger indices")
	flags.String(ilmPolicyName, "jaeger-ilm-policy", "The name of the ILM policy to use if ILM is active")
	flags.Int(timeout, 120, "Number of seconds to wait for master node response")
	flags.Bool(skipDependencies, false, "Disable rollover for dependencies index")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.IndexPrefix = v.GetString(indexPrefix)
	if c.IndexPrefix != "" {
		c.IndexPrefix += "-"
	}
	c.Archive = v.GetBool(archive)
	c.Username = v.GetString(username)
	c.Password = v.GetString(password)
	c.ILMPolicyName = v.GetString(ilmPolicyName)
	c.UseILM = v.GetBool(useILM)
	c.Timeout = v.GetInt(timeout)
	c.SkipDependencies = v.GetBool(skipDependencies)
}

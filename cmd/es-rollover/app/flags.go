// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

var tlsFlagsCfg = tlscfg.ClientFlagsConfig{Prefix: "es"}

const (
	indexPrefix      = "index-prefix"
	archive          = "archive"
	username         = "es.username"
	password         = "es.password"
	useILM           = "es.use-ilm"
	ilmPolicyName    = "es.ilm-policy-name"
	timeout          = "timeout"
	skipDependencies = "skip-dependencies"
	adaptiveSampling = "adaptive-sampling"
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
	AdaptiveSampling bool
	TLSConfig        configtls.ClientConfig
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
	flags.Bool(adaptiveSampling, false, "Enable rollover for adaptive sampling index")
	tlsFlagsCfg.AddFlags(flags)
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
	c.AdaptiveSampling = v.GetBool(adaptiveSampling)
	opts, err := tlsFlagsCfg.InitFromViper(v)
	if err != nil {
		panic(err)
	}
	c.TLSConfig = opts.ToOtelClientConfig()
}

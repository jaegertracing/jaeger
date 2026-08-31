// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/config/tlscfg"
)

var tlsFlagsCfg = tlscfg.ClientFlagsConfig{Prefix: "es"}

const (
	indexPrefix      = "index-prefix"
	archive          = "archive"
	username         = "es.username"
	password         = "es.password"
	tokenFile        = "es.token-file"   //nolint:gosec // G101: flag name, not a credential
	apiKeyFile       = "es.api-key-file" //nolint:gosec // G101: flag name, not a credential
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
	TokenFilePath    string
	APIKeyFilePath   string
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
	flags.String(tokenFile, "", "Path to a file containing bearer token")
	flags.String(apiKeyFile, "", "Path to a file containing API key")
	flags.Bool(useILM, false, "Use ILM to manage jaeger indices")
	flags.String(ilmPolicyName, "jaeger-ilm-policy", "The name of the ILM policy to use if ILM is active")
	flags.Int(timeout, 120, "Number of seconds to wait for master node response")
	flags.Bool(skipDependencies, false, "Disable rollover for dependencies index")
	flags.Bool(adaptiveSampling, false, "Enable rollover for adaptive sampling index")
	tlsFlagsCfg.AddFlags(flags)
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) error {
	c.IndexPrefix = v.GetString(indexPrefix)
	if c.IndexPrefix != "" {
		c.IndexPrefix += "-"
	}
	c.Archive = v.GetBool(archive)
	c.Username = v.GetString(username)
	c.Password = v.GetString(password)
	c.TokenFilePath = v.GetString(tokenFile)
	c.APIKeyFilePath = v.GetString(apiKeyFile)
	c.ILMPolicyName = v.GetString(ilmPolicyName)
	c.UseILM = v.GetBool(useILM)
	c.Timeout = v.GetInt(timeout)
	c.SkipDependencies = v.GetBool(skipDependencies)
	c.AdaptiveSampling = v.GetBool(adaptiveSampling)
	tlsCfg, err := tlsFlagsCfg.InitFromViper(v)
	if err != nil {
		return err
	}
	c.TLSConfig = tlsCfg
	return validateAuthFlags(c.Username, c.Password, c.TokenFilePath, c.APIKeyFilePath)
}

// validateAuthFlags rejects configuring more than one authentication method.
// The shared auth stack adds an Authorization header per configured method, so
// more than one would emit multiple Authorization headers, which ES/OS reject.
func validateAuthFlags(username, password, tokenFilePath, apiKeyFilePath string) error {
	basicAuth := username != "" && password != ""
	authMethods := 0
	for _, set := range []bool{basicAuth, tokenFilePath != "", apiKeyFilePath != ""} {
		if set {
			authMethods++
		}
	}
	if authMethods > 1 {
		return errors.New("only one of basic auth (--es.username/--es.password), --es.token-file, or --es.api-key-file may be configured")
	}
	return nil
}

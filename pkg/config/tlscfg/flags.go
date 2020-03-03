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

package tlscfg

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	tlsPrefix         = ".tls"
	tlsEnabledOld     = tlsPrefix
	tlsEnabled        = tlsPrefix + ".enabled"
	tlsCA             = tlsPrefix + ".ca"
	tlsCert           = tlsPrefix + ".cert"
	tlsKey            = tlsPrefix + ".key"
	tlsServerName     = tlsPrefix + ".server-name"
	tlsClientCA       = tlsPrefix + ".client-ca"
	tlsClientCAOld    = tlsPrefix + ".client.ca"
	tlsSkipHostVerify = tlsPrefix + ".skip-host-verify"
)

type Enabled int

const (
	Hide Enabled = iota
	Show
	ShowDeprecated
)

// ClientFlagsConfig describes which CLI flags for TLS client should be generated.
type ClientFlagsConfig struct {
	Prefix         string
	Enabled        Enabled
	ShowServerName bool
}

// ServerFlagsConfig describes which CLI flags for TLS server should be generated.
type ServerFlagsConfig struct {
	Prefix       string
	ShowEnabled  Enabled
	ShowClientCA bool
}

// AddFlags adds flags for TLS to the FlagSet.
func (c ClientFlagsConfig) AddFlags(flags *flag.FlagSet) {
	if c.Enabled >= Show {
		deprecated := ""
		if c.Enabled == ShowDeprecated {
			deprecated = "(deprecated)  "
		}
		flags.Bool(c.Prefix+tlsEnabled, false, deprecated+"Enable TLS when talking to the remote server(s)")
		flags.Bool(c.Prefix+tlsEnabledOld, false, "(deprecated) see --"+c.Prefix+tlsEnabled)
	}
	flags.String(c.Prefix+tlsCA, "", "Path to a TLS CA (Certification Authority) file used to verify the remote server(s) (by default will use the system truststore)")
	flags.String(c.Prefix+tlsCert, "", "Path to a TLS Certificate file, used to identify this process to the remote server(s)")
	flags.String(c.Prefix+tlsKey, "", "Path to a TLS Private Key file, used to identify this process to the remote server(s)")
	if c.ShowServerName {
		flags.String(c.Prefix+tlsServerName, "", "Override the TLS server name we expect in the certificate of the remove server(s)")
	}
	flags.Bool(c.Prefix+tlsSkipHostVerify, false, "(insecure) Skip server's certificate chain and host name verification")
}

// AddFlags adds flags for TLS to the FlagSet.
func (c ServerFlagsConfig) AddFlags(flags *flag.FlagSet) {
	if c.ShowEnabled >= Show {
		deprecated := ""
		if c.ShowEnabled == ShowDeprecated {
			deprecated = "(deprecated)  "
		}
		flags.Bool(c.Prefix+tlsEnabled, false, deprecated+"Enable TLS on the server")
		flags.Bool(c.Prefix+tlsEnabledOld, false, "(deprecated) see --"+c.Prefix+tlsEnabled)
	}
	flags.String(c.Prefix+tlsCert, "", "Path to a TLS Certificate file, used to identify this server to clients")
	flags.String(c.Prefix+tlsKey, "", "Path to a TLS Private Key file, used to identify this server to clients")
	flags.String(c.Prefix+tlsClientCA, "", "Path to a TLS CA (Certification Authority) file used to verify certificates presented by clients (if unset, all clients are permitted)")
	flags.String(c.Prefix+tlsClientCAOld, "", "(deprecated) see --"+c.Prefix+tlsClientCA)
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c ClientFlagsConfig) InitFromViper(v *viper.Viper) Options {
	var p Options
	if c.Enabled >= Show {
		p.Enabled = v.GetBool(c.Prefix + tlsEnabled)

		if !p.Enabled {
			p.Enabled = v.GetBool(c.Prefix + tlsEnabledOld)
		}
	}
	p.CAPath = v.GetString(c.Prefix + tlsCA)
	p.CertPath = v.GetString(c.Prefix + tlsCert)
	p.KeyPath = v.GetString(c.Prefix + tlsKey)
	if c.ShowServerName {
		p.ServerName = v.GetString(c.Prefix + tlsServerName)
	}
	p.SkipHostVerify = v.GetBool(c.Prefix + tlsSkipHostVerify)
	return p
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c ServerFlagsConfig) InitFromViper(v *viper.Viper) Options {
	var p Options
	if c.ShowEnabled >= Show {
		p.Enabled = v.GetBool(c.Prefix + tlsEnabled)

		if !p.Enabled {
			p.Enabled = v.GetBool(c.Prefix + tlsEnabledOld)
		}
	}
	p.CertPath = v.GetString(c.Prefix + tlsCert)
	p.KeyPath = v.GetString(c.Prefix + tlsKey)
	if c.ShowClientCA {
		p.ClientCAPath = v.GetString(c.Prefix + tlsClientCA)
		if s := v.GetString(c.Prefix + tlsClientCAOld); s != "" {
			// using legacy flag
			p.ClientCAPath = s
		}
	}
	return p
}

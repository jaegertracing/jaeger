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
	tlsEnabled     = "tls"
	tlsCA          = "tls.ca"
	tlsCert        = "tls.cert"
	tlsKey         = "tls.key"
	tlsServerName  = "tls.server-name"
	tlsClientCA    = "tls.client-ca"
	tlsClientCAOld = "tls.client.ca"
)

// FlagsConfig describes which CLI flags should be generated.
type FlagsConfig struct {
	Prefix         string
	ShowEnabled    bool
	ShowServerName bool
	ShowClientCA   bool
}

// AddFlags adds flags for TLS to the FlagSet.
func (c FlagsConfig) AddFlags(flags *flag.FlagSet) {
	if c.ShowEnabled {
		flags.Bool(c.Prefix+tlsEnabled, false, "Use TLS when talking to the remote server(s)")
	}
	flags.String(c.Prefix+tlsCA, "", "Path to a TLS CA (Certification Authority) file used to verify the remote(s) server (by default will use the system truststore)")
	flags.String(c.Prefix+tlsCert, "", "Path to a TLS Client Certificate file, used to identify this process to the remote server(s)")
	flags.String(c.Prefix+tlsKey, "", "Path to the TLS Private Key for the Client Certificate")
	if c.ShowServerName {
		flags.String(c.Prefix+tlsServerName, "", "Override the TLS server name we expect in the certificate of the remove server(s)")
	}
	if c.ShowClientCA {
		flags.String(c.Prefix+tlsClientCA, "", "Path to a TLS CA file used to verify certificates presented by clients (if unset, all clients are permitted)")
		flags.String(c.Prefix+tlsClientCAOld, "", "(deprecated) see --"+c.Prefix+tlsClientCA)
	}
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c FlagsConfig) InitFromViper(v *viper.Viper) Options {
	var p Options
	if c.ShowEnabled {
		p.Enabled = v.GetBool(c.Prefix + tlsEnabled)
	}
	p.CAPath = v.GetString(c.Prefix + tlsCA)
	p.CertPath = v.GetString(c.Prefix + tlsCert)
	p.KeyPath = v.GetString(c.Prefix + tlsKey)
	if c.ShowServerName {
		p.ServerName = v.GetString(c.Prefix + tlsServerName)
	}
	if c.ShowClientCA {
		p.ClientCAPath = v.GetString(c.Prefix + tlsClientCA)
		if s := v.GetString(c.Prefix + tlsClientCAOld); s != "" {
			// using legacy flag
			p.ClientCAPath = s
		}
	}
	return p
}

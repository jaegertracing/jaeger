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
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

const (
	tlsPrefix         = ".tls"
	tlsEnabled        = tlsPrefix + ".enabled"
	tlsCA             = tlsPrefix + ".ca"
	tlsCert           = tlsPrefix + ".cert"
	tlsKey            = tlsPrefix + ".key"
	tlsServerName     = tlsPrefix + ".server-name"
	tlsClientCA       = tlsPrefix + ".client-ca"
	tlsSkipHostVerify = tlsPrefix + ".skip-host-verify"
	tlsCipherSuites   = tlsPrefix + ".cipher-suites"
	tlsMinVersion     = tlsPrefix + ".min-version"
	tlsMaxVersion     = tlsPrefix + ".max-version"
)

// ClientFlagsConfig describes which CLI flags for TLS client should be generated.
type ClientFlagsConfig struct {
	Prefix string
}

// ServerFlagsConfig describes which CLI flags for TLS server should be generated.
type ServerFlagsConfig struct {
	Prefix string
}

// AddFlags adds flags for TLS to the FlagSet.
func (c ClientFlagsConfig) AddFlags(flags *flag.FlagSet) {
	flags.Bool(c.Prefix+tlsEnabled, false, "Enable TLS when talking to the remote server(s)")
	flags.String(c.Prefix+tlsCA, "", "Path to a TLS CA (Certification Authority) file used to verify the remote server(s) (by default will use the system truststore)")
	flags.String(c.Prefix+tlsCert, "", "Path to a TLS Certificate file, used to identify this process to the remote server(s)")
	flags.String(c.Prefix+tlsKey, "", "Path to a TLS Private Key file, used to identify this process to the remote server(s)")
	flags.String(c.Prefix+tlsServerName, "", "Override the TLS server name we expect in the certificate of the remote server(s)")
	flags.Bool(c.Prefix+tlsSkipHostVerify, false, "(insecure) Skip server's certificate chain and host name verification")
}

// AddFlags adds flags for TLS to the FlagSet.
func (c ServerFlagsConfig) AddFlags(flags *flag.FlagSet) {
	flags.Bool(c.Prefix+tlsEnabled, false, "Enable TLS on the server")
	flags.String(c.Prefix+tlsCert, "", "Path to a TLS Certificate file, used to identify this server to clients")
	flags.String(c.Prefix+tlsKey, "", "Path to a TLS Private Key file, used to identify this server to clients")
	flags.String(c.Prefix+tlsClientCA, "", "Path to a TLS CA (Certification Authority) file used to verify certificates presented by clients (if unset, all clients are permitted)")
	flags.String(c.Prefix+tlsCipherSuites, "", "Comma-separated list of cipher suites for the server, values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants).")
	flags.String(c.Prefix+tlsMinVersion, "", "Minimum TLS version supported (Possible values: 1.0, 1.1, 1.2, 1.3)")
	flags.String(c.Prefix+tlsMaxVersion, "", "Maximum TLS version supported (Possible values: 1.0, 1.1, 1.2, 1.3)")
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c ClientFlagsConfig) InitFromViper(v *viper.Viper) (Options, error) {
	var p Options
	p.Enabled = v.GetBool(c.Prefix + tlsEnabled)
	p.CAPath = v.GetString(c.Prefix + tlsCA)
	p.CertPath = v.GetString(c.Prefix + tlsCert)
	p.KeyPath = v.GetString(c.Prefix + tlsKey)
	p.ServerName = v.GetString(c.Prefix + tlsServerName)
	p.SkipHostVerify = v.GetBool(c.Prefix + tlsSkipHostVerify)

	if !p.Enabled {
		var empty Options
		if !reflect.DeepEqual(&p, &empty) {
			return p, fmt.Errorf("%s.tls.* options cannot be used when %s is false", c.Prefix, c.Prefix+tlsEnabled)
		}
	}

	return p, nil
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c ServerFlagsConfig) InitFromViper(v *viper.Viper) (Options, error) {
	var p Options
	p.Enabled = v.GetBool(c.Prefix + tlsEnabled)
	p.CertPath = v.GetString(c.Prefix + tlsCert)
	p.KeyPath = v.GetString(c.Prefix + tlsKey)
	p.ClientCAPath = v.GetString(c.Prefix + tlsClientCA)
	if s := v.GetString(c.Prefix + tlsCipherSuites); s != "" {
		p.CipherSuites = strings.Split(stripWhiteSpace(v.GetString(c.Prefix+tlsCipherSuites)), ",")
	}
	p.MinVersion = v.GetString(c.Prefix + tlsMinVersion)
	p.MaxVersion = v.GetString(c.Prefix + tlsMaxVersion)

	if !p.Enabled {
		var empty Options
		if !reflect.DeepEqual(&p, &empty) {
			return p, fmt.Errorf("%s.tls.* options cannot be used when %s is false", c.Prefix, c.Prefix+tlsEnabled)
		}
	}

	return p, nil
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}

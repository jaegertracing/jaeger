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

package auth

import (
	"flag"
	"strings"
)

const (
	suffixAuthentication = ".authentication"

	// Kerberos configuration options
	kerberosPrefix            = ".kerberos"
	suffixKerberosServiceName = ".service-name"
	suffixKerberosRealm       = ".realm"
	suffixKerberosUseKeyTab   = ".use-keytab"
	suffixKerberosUserName    = ".username"
	suffixKerberosPassword    = ".password"
	suffixKerberosConfig      = ".config-file"
	suffixKerberosKeyTab      = ".keytab-file"

	defaultAuthentication      = none
	defaultKerberosConfig      = "/etc/krb5.conf"
	defaultKerberosUseKeyTab   = false
	defaultKerberosServiceName = "kafka"
	defaultKerberosRealm       = ""
	defaultKerberosPassword    = ""
	defaultKerberosUsername    = ""
	defaultKerberosKeyTab      = "/etc/security/kafka.keytab"
)

func addKerberosFlags(configPrefix string, flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosServiceName,
		defaultKerberosServiceName,
		"Kerberos service name")
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosRealm,
		defaultKerberosRealm,
		"Kerberos realm")
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosPassword,
		defaultKerberosPassword,
		"The Kerberos password used for authenticate with KDC")
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosUserName,
		defaultKerberosUsername,
		"The Kerberos username used for authenticate with KDC")
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosConfig,
		defaultKerberosConfig,
		"Path to Kerberos configuration. i.e /etc/krb5.conf")
	flagSet.Bool(
		configPrefix+kerberosPrefix+suffixKerberosUseKeyTab,
		defaultKerberosUseKeyTab,
		"Use of keytab instead of password, if this is true, keytab file will be used instead of password")
	flagSet.String(
		configPrefix+kerberosPrefix+suffixKerberosKeyTab,
		defaultKerberosKeyTab,
		"Path to keytab file. i.e /etc/security/kafka.keytab")
}

// AddFlags add configuration flags to a flagSet.
func AddFlags(configPrefix string, flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixAuthentication,
		defaultAuthentication,
		"Authentication type used to authenticate with kafka cluster. e.g. "+strings.Join(authTypes, ", "),
	)
	addKerberosFlags(configPrefix, flagSet)
}

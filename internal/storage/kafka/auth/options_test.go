// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddFlags(t *testing.T) {
	tests := []struct {
		name          string
		configPrefix  string
		expectedFlags map[string]string
	}{
		{
			name:         "Default configuration with testprefix",
			configPrefix: "testprefix",
			expectedFlags: map[string]string{
				"testprefix" + suffixAuthentication:                       none,
				"testprefix" + kerberosPrefix + suffixKerberosServiceName: defaultKerberosServiceName,
				"testprefix" + kerberosPrefix + suffixKerberosRealm:       defaultKerberosRealm,
				"testprefix" + kerberosPrefix + suffixKerberosUsername:    defaultKerberosUsername,
				"testprefix" + kerberosPrefix + suffixKerberosPassword:    defaultKerberosPassword,
				"testprefix" + kerberosPrefix + suffixKerberosConfig:      defaultKerberosConfig,
				"testprefix" + kerberosPrefix + suffixKerberosUseKeyTab:   "false",
				"testprefix" + plainTextPrefix + suffixPlainTextUsername:  defaultPlainTextUsername,
				"testprefix" + plainTextPrefix + suffixPlainTextPassword:  defaultPlainTextPassword,
				"testprefix" + plainTextPrefix + suffixPlainTextMechanism: defaultPlainTextMechanism,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagSet := flag.NewFlagSet("test", flag.ContinueOnError)

			AddFlags(tt.configPrefix, flagSet)

			for flagName, expectedValue := range tt.expectedFlags {
				flag := flagSet.Lookup(flagName)
				assert.NotNil(t, flag, "Expected flag %q to be registered", flagName)
				assert.Equal(t, expectedValue, flag.DefValue, "Default value of flag %q", flagName)
			}
		})
	}
}

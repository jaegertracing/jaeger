// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestOptionsWithFlags(t *testing.T) {
	o := &Options{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--kafka.consumer.topic=topic1",
		"--kafka.consumer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.consumer.group-id=group1",
		"--kafka.consumer.client-id=client-id1",
		"--kafka.consumer.rack-id=rack1",
		"--kafka.consumer.fetch-max-message-bytes=10485760",
		"--kafka.consumer.encoding=json",
		"--kafka.consumer.protocol-version=1.0.0",
		"--ingester.parallelism=5",
		"--ingester.deadlockInterval=2m",
	})
	o.InitFromViper(v)

	assert.Equal(t, "topic1", o.Topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, o.Brokers)
	assert.Equal(t, "group1", o.GroupID)
	assert.Equal(t, "rack1", o.RackID)
	assert.Equal(t, int32(10485760), o.FetchMaxMessageBytes)
	assert.Equal(t, "client-id1", o.ClientID)
	assert.Equal(t, "1.0.0", o.ProtocolVersion)
	assert.Equal(t, 5, o.Parallelism)
	assert.Equal(t, 2*time.Minute, o.DeadlockInterval)
	assert.Equal(t, kafka.EncodingJSON, o.Encoding)
}

func TestTLSFlags(t *testing.T) {
	kerb := auth.KerberosConfig{ServiceName: "kafka", ConfigPath: "/etc/krb5.conf", KeyTabPath: "/etc/security/kafka.keytab"}
	plain := auth.PlainTextConfig{Username: "", Password: "", Mechanism: "PLAIN"}
	tests := []struct {
		flags    []string
		expected auth.AuthenticationConfig
	}{
		{
			flags:    []string{},
			expected: auth.AuthenticationConfig{Authentication: "none", Kerberos: kerb, PlainText: plain},
		},
		{
			flags:    []string{"--kafka.consumer.authentication=foo"},
			expected: auth.AuthenticationConfig{Authentication: "foo", Kerberos: kerb, PlainText: plain},
		},
		{
			flags:    []string{"--kafka.consumer.authentication=kerberos", "--kafka.consumer.tls.enabled=true"},
			expected: auth.AuthenticationConfig{Authentication: "kerberos", Kerberos: kerb, PlainText: plain},
		},
		{
			flags: []string{"--kafka.consumer.authentication=tls"},
			expected: auth.AuthenticationConfig{
				Authentication: "tls",
				Kerberos:       kerb,
				// TODO this test is unclear - if tls.enabled != true, why is it not tls.Insecure=true?
				TLS: configtls.ClientConfig{
					Config: configtls.Config{
						IncludeSystemCACertsPool: true,
					},
				},
				PlainText: plain,
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s", test.flags), func(t *testing.T) {
			o := &Options{}
			v, command := config.Viperize(AddFlags)
			err := command.ParseFlags(test.flags)
			require.NoError(t, err)
			o.InitFromViper(v)
			assert.Equal(t, test.expected, o.AuthenticationConfig)
		})
	}
}

func TestFlagDefaults(t *testing.T) {
	o := &Options{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	o.InitFromViper(v)

	assert.Equal(t, DefaultTopic, o.Topic)
	assert.Equal(t, []string{DefaultBroker}, o.Brokers)
	assert.Equal(t, DefaultGroupID, o.GroupID)
	assert.Equal(t, DefaultClientID, o.ClientID)
	assert.Equal(t, DefaultParallelism, o.Parallelism)
	assert.Equal(t, int32(DefaultFetchMaxMessageBytes), o.FetchMaxMessageBytes)
	assert.Equal(t, DefaultEncoding, o.Encoding)
	assert.Equal(t, DefaultDeadlockInterval, o.DeadlockInterval)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

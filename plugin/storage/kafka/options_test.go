// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"fmt"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--kafka.producer.topic=topic1",
		"--kafka.producer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.producer.encoding=protobuf",
		"--kafka.producer.required-acks=local",
		"--kafka.producer.compression=gzip",
		"--kafka.producer.compression-level=7",
		"--kafka.producer.batch-linger=1s",
		"--kafka.producer.batch-size=128000",
		"--kafka.producer.batch-min-messages=50",
		"--kafka.producer.batch-max-messages=100",
		"--kafka.producer.max-message-bytes=10485760",
	})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.Topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.Config.Brokers)
	assert.Equal(t, "protobuf", opts.Encoding)
	assert.Equal(t, sarama.WaitForLocal, opts.Config.RequiredAcks)
	assert.Equal(t, sarama.CompressionGZIP, opts.Config.Compression)
	assert.Equal(t, 7, opts.Config.CompressionLevel)
	assert.Equal(t, 128000, opts.Config.BatchSize)
	assert.Equal(t, time.Duration(1*time.Second), opts.Config.BatchLinger)
	assert.Equal(t, 50, opts.Config.BatchMinMessages)
	assert.Equal(t, 100, opts.Config.BatchMaxMessages)
	assert.Equal(t, 100, opts.Config.BatchMaxMessages)
	assert.Equal(t, 10485760, opts.Config.MaxMessageBytes)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultTopic, opts.Topic)
	assert.Equal(t, []string{defaultBroker}, opts.Config.Brokers)
	assert.Equal(t, defaultEncoding, opts.Encoding)
	assert.Equal(t, sarama.WaitForLocal, opts.Config.RequiredAcks)
	assert.Equal(t, sarama.CompressionNone, opts.Config.Compression)
	assert.Equal(t, 0, opts.Config.CompressionLevel)
	assert.Equal(t, 0, opts.Config.BatchSize)
	assert.Equal(t, time.Duration(0*time.Second), opts.Config.BatchLinger)
	assert.Equal(t, 0, opts.Config.BatchMinMessages)
	assert.Equal(t, 0, opts.Config.BatchMaxMessages)
	assert.Equal(t, defaultMaxMessageBytes, opts.Config.MaxMessageBytes)
}

func TestCompressionLevelDefaults(t *testing.T) {
	compressionLevel, err := getCompressionLevel("none", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["none"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("gzip", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["gzip"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("snappy", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["snappy"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("lz4", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["lz4"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("zstd", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["zstd"].defaultCompressionLevel, compressionLevel)
}

func TestCompressionLevel(t *testing.T) {
	compressionLevel, err := getCompressionLevel("none", 0)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["none"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("gzip", 4)
	require.NoError(t, err)
	assert.Equal(t, 4, compressionLevel)

	compressionLevel, err = getCompressionLevel("snappy", 0)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["snappy"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("lz4", 10)
	require.NoError(t, err)
	assert.Equal(t, 10, compressionLevel)

	compressionLevel, err = getCompressionLevel("zstd", 20)
	require.NoError(t, err)
	assert.Equal(t, 20, compressionLevel)
}

func TestFailedCompressionLevelScenario(t *testing.T) {
	_, err := getCompressionLevel("gzip", 14)
	require.Error(t, err)

	_, err = getCompressionLevel("lz4", 18)
	require.Error(t, err)

	_, err = getCompressionLevel("zstd", 25)
	require.Error(t, err)

	_, err = getCompressionLevel("test", 1)
	require.Error(t, err)
}

func TestCompressionModes(t *testing.T) {
	compressionModes, err := getCompressionMode("gzip")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionGZIP, compressionModes)

	compressionModes, err = getCompressionMode("snappy")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionSnappy, compressionModes)

	compressionModes, err = getCompressionMode("none")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionNone, compressionModes)
}

func TestCompressionModeFailures(t *testing.T) {
	_, err := getCompressionMode("test")
	require.Error(t, err)
}

func TestRequiredAcks(t *testing.T) {
	acks, err := getRequiredAcks("noack")
	require.NoError(t, err)
	assert.Equal(t, sarama.NoResponse, acks)

	acks, err = getRequiredAcks("local")
	require.NoError(t, err)
	assert.Equal(t, sarama.WaitForLocal, acks)

	acks, err = getRequiredAcks("all")
	require.NoError(t, err)
	assert.Equal(t, sarama.WaitForAll, acks)
}

func TestRequiredAcksFailures(t *testing.T) {
	_, err := getRequiredAcks("test")
	require.Error(t, err)
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
			flags:    []string{"--kafka.producer.authentication=foo"},
			expected: auth.AuthenticationConfig{Authentication: "foo", Kerberos: kerb, PlainText: plain},
		},
		{
			flags:    []string{"--kafka.producer.authentication=kerberos", "--kafka.producer.tls.enabled=true"},
			expected: auth.AuthenticationConfig{Authentication: "kerberos", Kerberos: kerb, TLS: configtls.ClientConfig{}, PlainText: plain},
		},
		{
			flags: []string{"--kafka.producer.authentication=tls"},
			expected: auth.AuthenticationConfig{
				Authentication: "tls",
				Kerberos:       kerb,
				TLS: configtls.ClientConfig{
					Config: configtls.Config{
						IncludeSystemCACertsPool: true,
					},
				},
				PlainText: plain,
			},
		},
		{
			flags: []string{"--kafka.producer.authentication=tls"},
			expected: auth.AuthenticationConfig{
				Authentication: "tls",
				Kerberos:       kerb,
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
			v, command := config.Viperize(o.AddFlags)
			err := command.ParseFlags(test.flags)
			require.NoError(t, err)
			o.InitFromViper(v)
			assert.Equal(t, test.expected, o.Config.AuthenticationConfig)
		})
	}
}

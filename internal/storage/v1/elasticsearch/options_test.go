// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
	escfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetConfig()
	assert.Empty(t, primary.Authentication.BasicAuthentication.Username)
	assert.Empty(t, primary.Authentication.BasicAuthentication.Password)
	assert.Empty(t, primary.Authentication.BasicAuthentication.PasswordFilePath)
	assert.NotEmpty(t, primary.Servers)
	assert.Empty(t, primary.RemoteReadClusters)
	assert.EqualValues(t, 5, primary.Indices.Spans.Shards)
	assert.EqualValues(t, 5, primary.Indices.Services.Shards)
	assert.EqualValues(t, 5, primary.Indices.Sampling.Shards)
	assert.EqualValues(t, 5, primary.Indices.Dependencies.Shards)
	assert.EqualValues(t, 1, primary.Indices.Spans.Replicas)
	assert.EqualValues(t, 1, primary.Indices.Services.Replicas)
	assert.EqualValues(t, 1, primary.Indices.Sampling.Replicas)
	assert.EqualValues(t, 1, primary.Indices.Dependencies.Replicas)
	assert.Equal(t, 72*time.Hour, primary.MaxSpanAge)
	assert.False(t, primary.Sniffing.Enabled)
	assert.False(t, primary.Sniffing.UseHTTPS)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("es")
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--es.server-urls=1.1.1.1, 2.2.2.2",
		"--es.username=hello",
		"--es.password=world",
		"--es.token-file=/foo/bar",
		"--es.password-file=/foo/bar/baz",
		"--es.sniffer=true",
		"--es.sniffer-tls-enabled=true",
		"--es.max-span-age=48h",
		"--es.num-shards=20",
		"--es.num-replicas=10",
		"--es.index-date-separator=",
		"--es.index-rollover-frequency-spans=hour",
		"--es.index-rollover-frequency-services=day",
		// a couple overrides
		"--es.remote-read-clusters=cluster_one,cluster_two",
		"--es.tls.enabled=true",
		"--es.tls.skip-host-verify=true",
		"--es.tags-as-fields.all=true",
		"--es.tags-as-fields.include=test,tags",
		"--es.tags-as-fields.config-file=./file.txt",
		"--es.tags-as-fields.dot-replacement=!",
		"--es.use-ilm=true",
		"--es.send-get-body-as=POST",
	})
	require.NoError(t, err)
	opts.InitFromViper(v)

	primary := opts.GetConfig()
	assert.Equal(t, "hello", primary.Authentication.BasicAuthentication.Username)
	assert.Equal(t, "world", primary.Authentication.BasicAuthentication.Password)
	assert.Equal(t, "/foo/bar", primary.Authentication.BearerTokenAuthentication.FilePath)
	assert.Equal(t, "/foo/bar/baz", primary.Authentication.BasicAuthentication.PasswordFilePath)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, []string{"cluster_one", "cluster_two"}, primary.RemoteReadClusters)
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	assert.True(t, primary.Sniffing.Enabled)
	assert.True(t, primary.Sniffing.UseHTTPS)
	assert.False(t, primary.TLS.Insecure)
	assert.True(t, primary.TLS.InsecureSkipVerify)
	assert.True(t, primary.Tags.AllAsFields)
	assert.Equal(t, "!", primary.Tags.DotReplacement)
	assert.Equal(t, "./file.txt", primary.Tags.File)
	assert.Equal(t, "test,tags", primary.Tags.Include)
	assert.Equal(t, "20060102", primary.Indices.Services.DateLayout)
	assert.Equal(t, "2006010215", primary.Indices.Spans.DateLayout)
	assert.True(t, primary.UseILM)
}

func TestEmptyRemoteReadClusters(t *testing.T) {
	opts := NewOptions("es")
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--es.remote-read-clusters=",
	})
	require.NoError(t, err)
	opts.InitFromViper(v)

	primary := opts.GetConfig()
	assert.Equal(t, []string{}, primary.RemoteReadClusters)
}

func TestMaxSpanAgeSetErrorInArchiveMode(t *testing.T) {
	opts := NewOptions(archiveNamespace)
	_, command := config.Viperize(opts.AddFlags)
	flags := []string{"--es-archive.max-span-age=24h"}
	err := command.ParseFlags(flags)
	require.EqualError(t, err, "unknown flag: --es-archive.max-span-age")
}

func TestMaxDocCount(t *testing.T) {
	testCases := []struct {
		name            string
		flags           []string
		wantMaxDocCount int
	}{
		{"neither defined", []string{}, 10_000},
		{"max-doc-count only", []string{"--es.max-doc-count=1000"}, 1000},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)

			primary := opts.GetConfig()
			assert.Equal(t, tc.wantMaxDocCount, primary.MaxDocCount)
		})
	}
}

func TestIndexDateSeparator(t *testing.T) {
	testCases := []struct {
		name           string
		flags          []string
		wantDateLayout string
	}{
		{"not defined (default)", []string{}, "2006-01-02"},
		{"empty separator", []string{"--es.index-date-separator="}, "20060102"},
		{"dot separator", []string{"--es.index-date-separator=."}, "2006.01.02"},
		{"crossbar separator", []string{"--es.index-date-separator=-"}, "2006-01-02"},
		{"slash separator", []string{"--es.index-date-separator=/"}, "2006/01/02"},
		{"empty string with single quotes", []string{"--es.index-date-separator=''"}, "2006''01''02"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)

			primary := opts.GetConfig()
			assert.Equal(t, tc.wantDateLayout, primary.Indices.Spans.DateLayout)
		})
	}
}

func TestIndexRollover(t *testing.T) {
	testCases := []struct {
		name                              string
		flags                             []string
		wantSpanDateLayout                string
		wantServiceDateLayout             string
		wantSpanIndexRolloverFrequency    time.Duration
		wantServiceIndexRolloverFrequency time.Duration
	}{
		{
			name:                              "not defined (default)",
			flags:                             []string{},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name:                              "index day rollover",
			flags:                             []string{"--es.index-rollover-frequency-services=day", "--es.index-rollover-frequency-spans=hour"},
			wantSpanDateLayout:                "2006-01-02-15",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -1 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name:                              "index hour rollover",
			flags:                             []string{"--es.index-rollover-frequency-services=hour", "--es.index-rollover-frequency-spans=day"},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02-15",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -1 * time.Hour,
		},
		{
			name:                              "invalid index rollover frequency falls back to default 'day'",
			flags:                             []string{"--es.index-rollover-frequency-services=hours", "--es.index-rollover-frequency-spans=hours"},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)
			primary := opts.GetConfig()
			assert.Equal(t, tc.wantSpanDateLayout, primary.Indices.Spans.DateLayout)
			assert.Equal(t, tc.wantServiceDateLayout, primary.Indices.Services.DateLayout)
			assert.Equal(t, tc.wantSpanIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(primary.Indices.Spans.RolloverFrequency))
			assert.Equal(t, tc.wantServiceIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(primary.Indices.Services.RolloverFrequency))
		})
	}
}

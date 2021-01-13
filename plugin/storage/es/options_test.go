// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package es

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetPrimary()
	assert.Empty(t, primary.Username)
	assert.Empty(t, primary.Password)
	assert.NotEmpty(t, primary.Servers)
	assert.Equal(t, int64(5), primary.NumShards)
	assert.Equal(t, int64(1), primary.NumReplicas)
	assert.Equal(t, 72*time.Hour, primary.MaxSpanAge)
	assert.False(t, primary.Sniffer)
	assert.False(t, primary.SnifferTLSEnabled)

	aux := opts.Get("archive")
	assert.Equal(t, primary.Username, aux.Username)
	assert.Equal(t, primary.Password, aux.Password)
	assert.Equal(t, primary.Servers, aux.Servers)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("es", "es.aux")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--es.server-urls=1.1.1.1, 2.2.2.2",
		"--es.username=hello",
		"--es.password=world",
		"--es.token-file=/foo/bar",
		"--es.sniffer=true",
		"--es.sniffer-tls-enabled=true",
		"--es.max-span-age=48h",
		"--es.num-shards=20",
		"--es.num-replicas=10",
		"--es.index-date-separator=",
		// a couple overrides
		"--es.aux.server-urls=3.3.3.3, 4.4.4.4",
		"--es.aux.max-span-age=24h",
		"--es.aux.num-replicas=10",
		"--es.aux.index-date-separator=.",
		"--es.tls.enabled=true",
		"--es.tls.skip-host-verify=true",
		"--es.tags-as-fields.all=true",
		"--es.tags-as-fields.include=test,tags",
		"--es.tags-as-fields.config-file=./file.txt",
		"--es.tags-as-fields.dot-replacement=!",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
	assert.Equal(t, "hello", primary.Username)
	assert.Equal(t, "/foo/bar", primary.TokenFilePath)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	assert.True(t, primary.Sniffer)
	assert.True(t, primary.SnifferTLSEnabled)
	assert.Equal(t, true, primary.TLS.Enabled)
	assert.Equal(t, true, primary.TLS.SkipHostVerify)
	assert.True(t, primary.Tags.AllAsFields)
	assert.Equal(t, "!", primary.Tags.DotReplacement)
	assert.Equal(t, "./file.txt", primary.Tags.File)
	assert.Equal(t, "test,tags", primary.Tags.Include)
	assert.Equal(t, "20060102", primary.IndexDateLayout)

	aux := opts.Get("es.aux")
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Servers)
	assert.Equal(t, "hello", aux.Username)
	assert.Equal(t, "world", aux.Password)
	assert.Equal(t, int64(20), aux.NumShards)
	assert.Equal(t, int64(10), aux.NumReplicas)
	assert.Equal(t, 24*time.Hour, aux.MaxSpanAge)
	assert.True(t, aux.Sniffer)
	assert.True(t, aux.Tags.AllAsFields)
	assert.Equal(t, "!", aux.Tags.DotReplacement)
	assert.Equal(t, "./file.txt", aux.Tags.File)
	assert.Equal(t, "test,tags", aux.Tags.Include)
	assert.Equal(t, "2006.01.02", aux.IndexDateLayout)
}

func TestMaxNumSpansUsage(t *testing.T) {
	testCases := []struct {
		namespace string
		wantUsage string
	}{
		{
			namespace: "es",
			wantUsage: "(deprecated, will be removed in release v1.21.0. Please use es.max-doc-count). " +
				"The maximum number of spans to fetch at a time per query in Elasticsearch. " +
				"The lesser of es.max-num-spans and es.max-doc-count will be used if both are set.",
		},
		{
			namespace: "es-archive",
			wantUsage: "(deprecated, will be removed in release v1.21.0. Please use es-archive.max-doc-count). " +
				"The maximum number of spans to fetch at a time per query in Elasticsearch. " +
				"The lesser of es-archive.max-num-spans and es-archive.max-doc-count will be used if both are set.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.namespace, func(t *testing.T) {
			opts := NewOptions(tc.namespace)
			_, command := config.Viperize(opts.AddFlags)
			assert.Equal(t, tc.wantUsage, command.Flag(tc.namespace+".max-num-spans").Usage)
		})
	}
}

func TestMaxDocCount(t *testing.T) {
	testCases := []struct {
		name            string
		flags           []string
		wantMaxDocCount int
	}{
		{"neither defined", []string{}, 10_000},
		{"max-num-spans only", []string{"--es.max-num-spans=1000"}, 1000},
		{"max-doc-count only", []string{"--es.max-doc-count=1000"}, 1000},
		{"max-num-spans == max-doc-count", []string{"--es.max-num-spans=1000", "--es.max-doc-count=1000"}, 1000},
		{"max-num-spans < max-doc-count", []string{"--es.max-num-spans=999", "--es.max-doc-count=1000"}, 999},
		{"max-num-spans > max-doc-count", []string{"--es.max-num-spans=1000", "--es.max-doc-count=999"}, 999},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es", "es.aux")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)

			primary := opts.GetPrimary()
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

			primary := opts.GetPrimary()
			assert.Equal(t, tc.wantDateLayout, primary.IndexDateLayout)
		})
	}
}

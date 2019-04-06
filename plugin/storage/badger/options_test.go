// Copyright (c) 2018 The Jaeger Authors.
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

package badger

import (
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultOptionsParsing(t *testing.T) {
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.True(t, opts.GetPrimary().Ephemeral)
	assert.False(t, opts.GetPrimary().SyncWrites)
	assert.Equal(t, time.Duration(72*time.Hour), opts.GetPrimary().SpanStoreTTL)
}

func TestParseOptions(t *testing.T) {
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=false",
		"--badger.consistency=true",
		"--badger.directory-key=/var/lib/badger",
		"--badger.directory-value=/mnt/slow/badger",
		"--badger.span-store-ttl=168h",
	})
	opts.InitFromViper(v)

	assert.False(t, opts.GetPrimary().Ephemeral)
	assert.True(t, opts.GetPrimary().SyncWrites)
	assert.Equal(t, time.Duration(168*time.Hour), opts.GetPrimary().SpanStoreTTL)
	assert.Equal(t, "/var/lib/badger", opts.GetPrimary().KeyDirectory)
	assert.Equal(t, "/mnt/slow/badger", opts.GetPrimary().ValueDirectory)
}

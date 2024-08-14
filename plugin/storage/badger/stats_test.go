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

//go:build !linux
// +build !linux

package badger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func TestDiskStatisticsUpdate(t *testing.T) {
	f := NewFactory()
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v, zap.NewNop())
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	// We're not expecting any value in !linux, just no error
	err = f.diskStatisticsUpdate()
	require.NoError(t, err)
}

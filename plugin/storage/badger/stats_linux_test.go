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

	assert "github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestDiskStatisticsUpdate(t *testing.T) {
	f := NewFactory()
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v)
	mFactory := metricstest.NewFactory(0)
	err := f.Initialize(mFactory, zap.NewNop())
	assert.NoError(t, err)
	defer f.Close()

	err = f.diskStatisticsUpdate()
	assert.NoError(t, err)
	_, gs := mFactory.Snapshot()
	assert.True(t, gs[keyLogSpaceAvailableName] > int64(0))
	assert.True(t, gs[valueLogSpaceAvailableName] > int64(0))
}

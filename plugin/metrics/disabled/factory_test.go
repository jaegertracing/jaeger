// Copyright (c) 2021 The Jaeger Authors.
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

package disabled

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.MetricsFactory = new(Factory)

func TestPrometheusFactory(t *testing.T) {
	f := NewFactory()
	assert.NoError(t, f.Initialize(zap.NewNop()))

	err := f.Initialize(nil)
	require.NoError(t, err)

	f.AddFlags(nil)
	f.InitFromViper(nil, zap.NewNop())

	reader, err := f.CreateMetricsReader()
	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

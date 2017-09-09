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

package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/pkg/es/mocks"
)

type mockEsBuilder struct {
	config.Configuration
}

func (mck *mockEsBuilder) NewClient() (es.Client, error) {
	return &mocks.Client{}, nil
}

func TestNewESBuilderSuccess(t *testing.T) {
	sb := newStorageBuilder()
	err := sb.newESBuilder(&mockEsBuilder{})
	require.NoError(t, err)
	assert.NotNil(t, sb.SpanReader)
	assert.NotNil(t, sb.DependencyReader)
}

func TestNewESBuilderFailure(t *testing.T) {
	sb := newStorageBuilder()
	err := sb.newESBuilder(&config.Configuration{})
	require.Error(t, err)
	require.Nil(t, sb.SpanReader)
	require.Nil(t, sb.DependencyReader)
}

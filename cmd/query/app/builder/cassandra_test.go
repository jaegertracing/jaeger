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

	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/cassandra"
	"github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
)

type mockSessionBuilder struct {
}

func (*mockSessionBuilder) NewSession() (cassandra.Session, error) {
	return &mocks.Session{}, nil
}

func TestNewBuilderFailure(t *testing.T) {
	sFlags := &flags.SharedFlags{}
	sb := newStorageBuilder()
	err := sb.newCassandraBuilder(&config.Configuration{}, sFlags.DependencyStorage.DataFrequency)
	require.Error(t, err)
	assert.Nil(t, sb.SpanReader)
	assert.Nil(t, sb.DependencyReader)
}

func TestNewBuilderSuccess(t *testing.T) {
	sFlags := &flags.SharedFlags{}

	sb := newStorageBuilder()
	err := sb.newCassandraBuilder(&mockSessionBuilder{}, sFlags.DependencyStorage.DataFrequency)
	require.NoError(t, err)
	assert.NotNil(t, sb.SpanReader)
	assert.NotNil(t, sb.DependencyReader)
}

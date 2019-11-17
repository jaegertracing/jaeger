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

package spanstore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

func TestTraceVersion_V1(t *testing.T) {
	versionQuery := func() *mocks.Query {
		query := &mocks.Query{}
		query.On("Exec").Return(errors.New("version read error"))
		return query
	}

	mockSession := &mocks.Session{}
	mockSession.On("Query", stringMatcher(queryTraceTableVersion), matchEverything()).Return(versionQuery())
	version := getTraceTableVersion(context.Background(), mockSession)
	assert.EqualValues(t, dbmodel.V1, version)
}

func TestTraceVersion_V2(t *testing.T) {
	versionQuery := func() *mocks.Query {
		query := &mocks.Query{}
		query.On("Exec").Return(nil)
		return query
	}

	mockSession := &mocks.Session{}
	mockSession.On("Query", stringMatcher(queryTraceTableVersion), matchEverything()).Return(versionQuery())
	version := getTraceTableVersion(context.Background(), mockSession)
	assert.EqualValues(t, dbmodel.V2, version)
}

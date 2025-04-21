// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
)

func TestGetDependencyVersionV1(t *testing.T) {
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(errors.New("error"))

	assert.Equal(t, V1, GetDependencyVersion(session))
}

func TestGetDependencyVersionV2(t *testing.T) {
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	assert.Equal(t, V2, GetDependencyVersion(session))
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"fmt"

	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
)

func GetSessionWithError(err error) *mocks.Session {
	session := &mocks.Session{}
	query := &mocks.Query{}
	query.On("Exec").Return(err)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName),
		mock.Anything).Return(query)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[previousVersion].tableName),
		mock.Anything).Return(query)
	return session
}

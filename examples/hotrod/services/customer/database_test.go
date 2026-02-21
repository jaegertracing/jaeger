// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package customer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/httperr"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
)

func TestDatabaseGetValidCustomer(t *testing.T) {
	logger := log.NewFactory(zap.NewNop())
	db := newDatabase(trace.NewNoopTracerProvider().Tracer("test"), logger)

	// Test getting a valid customer
	customer, err := db.Get(context.Background(), 123)
	require.NoError(t, err)
	assert.NotNil(t, customer)
	assert.Equal(t, "123", customer.ID)
	assert.Equal(t, "Rachel's_Floral_Designs", customer.Name)
}

func TestDatabaseGetInvalidCustomer(t *testing.T) {
	logger := log.NewFactory(zap.NewNop())
	db := newDatabase(trace.NewNoopTracerProvider().Tracer("test"), logger)

	// Test getting an invalid customer
	customer, err := db.Get(context.Background(), 999)
	assert.Nil(t, customer)
	require.Error(t, err)

	// Verify it's a BadRequestError (user error, not server error)
	var badReqErr *httperr.BadRequestError
	require.ErrorAs(t, err, &badReqErr)
	assert.Contains(t, err.Error(), "invalid customer ID")
}

func TestDatabaseGetMultipleValidCustomers(t *testing.T) {
	logger := log.NewFactory(zap.NewNop())
	db := newDatabase(trace.NewNoopTracerProvider().Tracer("test"), logger)

	// Test that all valid customers can be retrieved
	customer123, err := db.Get(context.Background(), 123)
	require.NoError(t, err)
	assert.NotNil(t, customer123)
	assert.Equal(t, "123", customer123.ID)

	customer567, err := db.Get(context.Background(), 567)
	require.NoError(t, err)
	assert.NotNil(t, customer567)
	assert.Equal(t, "567", customer567.ID)
}

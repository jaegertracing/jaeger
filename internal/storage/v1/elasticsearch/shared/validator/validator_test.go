// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestTraceQueryParameterValidation(t *testing.T) {
	tqp := dbmodel.TraceQueryParameters{
		ServiceName: "",
		Tags: map[string]string{
			"hello": "world",
		},
	}
	err := ValidateQuery(tqp)
	require.EqualError(t, err, ErrServiceNameNotSet.Error())

	tqp.ServiceName = "test-service"

	tqp.StartTimeMin = time.Time{} // time.Unix(0,0) doesn't work because timezones
	tqp.StartTimeMax = time.Time{}
	err = ValidateQuery(tqp)
	require.EqualError(t, err, ErrStartAndEndTimeNotSet.Error())

	tqp.StartTimeMin = time.Now()
	tqp.StartTimeMax = time.Now().Add(-1 * time.Hour)
	err = ValidateQuery(tqp)
	require.EqualError(t, err, ErrStartTimeMinGreaterThanMax.Error())

	tqp.StartTimeMin = time.Now().Add(-1 * time.Hour)
	tqp.StartTimeMax = time.Now()
	err = ValidateQuery(tqp)
	require.NoError(t, err)

	tqp.DurationMin = time.Hour
	tqp.DurationMax = time.Minute
	err = ValidateQuery(tqp)
	require.EqualError(t, err, ErrDurationMinGreaterThanMax.Error())
}

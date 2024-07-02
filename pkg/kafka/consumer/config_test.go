// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSetConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	test := &Configuration{AuthenticationConfig: auth.AuthenticationConfig{Authentication: "fail"}}
	_, err := test.NewConsumer(logger)
	require.EqualError(t, err, "Unknown/Unsupported authentication method fail to kafka cluster")
}

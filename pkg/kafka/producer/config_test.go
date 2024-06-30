// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSetConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	test := &Configuration{AuthenticationConfig: auth.AuthenticationConfig{Authentication: "fail"}}
	_, err := test.NewProducer(logger)
	require.EqualError(t, err, "Unknown/Unsupported authentication method fail to kafka cluster")
}

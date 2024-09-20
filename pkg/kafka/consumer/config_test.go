// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSetConfiguration(t *testing.T) {
	test := &Configuration{AuthenticationConfig: auth.AuthenticationConfig{Authentication: "fail"}}
	_, err := test.NewConsumer(context.Background())
	require.EqualError(t, err, "Unknown/Unsupported authentication method fail to kafka cluster")
}

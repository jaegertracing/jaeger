// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSetConfiguration(t *testing.T) {
	test := &Configuration{AuthenticationConfig: auth.AuthenticationConfig{Authentication: "fail"}}
	_, err := test.NewProducer(context.Background())
	require.ErrorIs(t, err, auth.ErrUnsupportedAuthMethod)
}

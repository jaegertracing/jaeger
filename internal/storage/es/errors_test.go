// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"errors"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"
)

func TestDetailedError(t *testing.T) {
	require.ErrorContains(t, errors.New("some err"), "some err", "no panic")

	esErr := &elastic.Error{
		Status: 400,
		Details: &elastic.ErrorDetails{
			Type:   "type1",
			Reason: "useless reason, e.g. all shards failed",
			RootCause: []*elastic.ErrorDetails{
				{
					Type:   "type2",
					Reason: "actual reason",
				},
			},
		},
	}
	require.ErrorContains(t, DetailedError(esErr), "actual reason")

	esErr.Details.RootCause[0] = nil
	require.ErrorContains(t, DetailedError(esErr), "useless reason")
	require.NotContains(t, DetailedError(esErr).Error(), "actual reason")
}

// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"fmt"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
)

func TestDetailedError(t *testing.T) {
	assert.ErrorContains(t, fmt.Errorf("some err"), "some err", "no panic")

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
	assert.ErrorContains(t, DetailedError(esErr), "actual reason")

	esErr.Details.RootCause[0] = nil
	assert.ErrorContains(t, DetailedError(esErr), "useless reason")
	assert.NotContains(t, DetailedError(esErr).Error(), "actual reason")
}

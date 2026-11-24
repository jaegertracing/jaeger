// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestConvertDependencies(t *testing.T) {
	tests := []struct {
		dLinks   []model.DependencyLink
		expected []model.DependencyLink
	}{
		{
			dLinks: []model.DependencyLink{
				{
					Parent:    "foo",
					Child:     "bar",
					CallCount: 1,
					Source:    "mesh",
				},
			},
			expected: []model.DependencyLink{
				{
					Parent:    "foo",
					Child:     "bar",
					CallCount: 1,
					Source:    "mesh",
				},
			},
		},
		{
			dLinks: []model.DependencyLink{
				{
					Parent:    "foo",
					CallCount: 3,
				},
			},
			expected: []model.DependencyLink{
				{
					Parent:    "foo",
					CallCount: 3,
					Source:    model.JaegerDependencyLinkSource,
				},
			},
		},
		{
			dLinks:   []model.DependencyLink{},
			expected: []model.DependencyLink{},
		},
		{
			dLinks:   nil,
			expected: nil,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := FromDomainDependencies(test.dLinks)
			a := ToDomainDependencies(got)
			assert.Equal(t, test.expected, a)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

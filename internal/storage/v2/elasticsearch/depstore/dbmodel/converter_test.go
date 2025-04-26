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
		dLinks []model.DependencyLink
	}{
		{
			dLinks: []model.DependencyLink{{CallCount: 1, Parent: "foo", Child: "bar"}},
		},
		{
			dLinks: []model.DependencyLink{{CallCount: 3, Parent: "foo"}},
		},
		{
			dLinks: []model.DependencyLink{},
		},
		{
			dLinks: nil,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := FromDomainDependencies(test.dLinks)
			a := ToDomainDependencies(got)
			assert.Equal(t, test.dLinks, a)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

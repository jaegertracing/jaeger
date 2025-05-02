// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package hostname

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestAsIdentifier(t *testing.T) {
	var hostname1 string
	var hostname2 string

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		var err error
		hostname1, err = AsIdentifier()
		assert.NoError(t, err)
		wg.Done()
	}()
	go func() {
		var err error
		hostname2, err = AsIdentifier()
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()

	actualHostname, _ := os.Hostname()
	assert.Equal(t, hostname1, hostname2)
	assert.True(t, strings.HasPrefix(hostname1, actualHostname))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCronJob_Start(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)
	var mu sync.Mutex
	counter := 0
	job := NewCronJob(1*time.Second, func() {
		mu.Lock()
		counter++
		mu.Unlock()
	})
	job.Start()
	time.Sleep(1 * time.Second)
	job.Close()
	assert.NotNil(t, job)
	mu.Lock()
	assert.Equal(t, 1, counter)
	mu.Unlock()
}

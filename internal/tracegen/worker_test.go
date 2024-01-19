// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func Test_SimulateTraces(t *testing.T) {
	logger, buf := testutils.NewLogger()
	tp := sdktrace.NewTracerProvider()
	tracers := []trace.Tracer{tp.Tracer("stdout")}
	wg := sync.WaitGroup{}
	wg.Add(1)
	var running uint32 = 1

	worker := &worker{
		logger:  logger,
		tracers: tracers,
		wg:      &wg,
		id:      7,
		running: &running,
		Config: Config{
			Traces:   7,
			Duration: time.Second,
			Pause:    time.Second,
			Service:  "stdout",
			Debug:    true,
			Firehose: true,
		},
	}
	expectedOutput := `{"level":"info","msg":"Worker 7 generated 7 traces"}` + "\n"

	worker.simulateTraces()
	assert.Equal(t, expectedOutput, buf.String())
}

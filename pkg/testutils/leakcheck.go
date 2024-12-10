// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"testing"

	"go.uber.org/goleak"
)

// IgnoreGlogFlushDaemonLeak returns a goleak.Option that ignores the flushDaemon function
// from the glog package that can cause false positives in leak detection.
// This is necessary because glog starts a goroutine in the background that may not
// be stopped when the test finishes, leading to a detected but expected leak.
func IgnoreGlogFlushDaemonLeak() goleak.Option {
	return goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon")
}

// IgnoreOpenCensusWorkerLeak This prevent catching the leak generated by opencensus defaultWorker.Start which at the time is part of the package's init call.
// See https://github.com/jaegertracing/jaeger/pull/5055#discussion_r1438702168 for more context.
func IgnoreOpenCensusWorkerLeak() goleak.Option {
	return goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start")
}

// IgnoreGoMetricsMeterLeak prevents the leak created by go-metrics which is being used by Sarama (Kafka Client) in Jaeger v1. This reason of this leak is
// not Jaeger but the go-metrics used by Samara.
// See these issues for the context
// https://github.com/IBM/sarama/issues/1321,
// https://github.com/IBM/sarama/issues/1340, and
// https://github.com/IBM/sarama/issues/2832
func IgnoreGoMetricsMeterLeak() goleak.Option {
	return goleak.IgnoreTopFunction("github.com/rcrowley/go-metrics.(*meterArbiter).tick")
}

// VerifyGoLeaks verifies that unit tests do not leak any goroutines.
// It should be called in TestMain.
func VerifyGoLeaks(m *testing.M) {
	goleak.VerifyTestMain(m, IgnoreGlogFlushDaemonLeak(), IgnoreOpenCensusWorkerLeak())
}

// VerifyGoLeaksOnce verifies that a given unit test does not leak any goroutines.
// Occasionally useful to troubleshoot specific tests that are flaky due to leaks,
// since VerifyGoLeaks cannot distiguish which specific test caused the leak.
// It should be called via defer or from Cleanup:
//
//	defer testutils.VerifyGoLeaksOnce(t)
func VerifyGoLeaksOnce(t *testing.T) {
	goleak.VerifyNone(t, IgnoreGlogFlushDaemonLeak(), IgnoreOpenCensusWorkerLeak(), IgnoreGoMetricsMeterLeak())
}

// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutils

import (
	"testing"

	"go.uber.org/goleak"
)

// TolerantVerifyLeakMain verifies go leaks but excludes the go routines that are
// launched as side effects of some of our dependencies.
func TolerantVerifyLeakMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// https://github.com/census-instrumentation/opencensus-go/blob/d7677d6af5953e0506ac4c08f349c62b917a443a/stats/view/worker.go#L34
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		// https://github.com/kubernetes/klog/blob/c85d02d1c76a9ebafa81eb6d35c980734f2c4727/klog.go#L417
		goleak.IgnoreTopFunction("k8s.io/klog/v2.(*loggingT).flushDaemon"),
		goleak.IgnoreTopFunction("k8s.io/klog.(*loggingT).flushDaemon"),
		// https://github.com/dgraph-io/badger/blob/v1.5.3/y/watermark.go#L54
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger/y.(*WaterMark).process"),
		// https://github.com/dgraph-io/badger/blob/v1.5.3/db.go#L264
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger.(*DB).updateSize"),
		// https://github.com/dgraph-io/badger/blob/v1.5.3/levels.go#L177
		goleak.IgnoreTopFunction("time.Sleep"),
		// https://github.com/dgraph-io/badger/blob/v1.5.3/levels.go#L177
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger.(*levelsController).runWorker"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger.(*DB).flushMemtable"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger.(*DB).doWrites"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/badger.(*valueLog).waitOnGC"),
		goleak.IgnoreTopFunction("github.com/jaegertracing/jaeger/plugin/storage/badger.(*Factory).maintenance"),
		goleak.IgnoreTopFunction("github.com/jaegertracing/jaeger/plugin/storage/badger.(*Factory).metricsCopier"),
	)
}

// Copyright (c) 2018 The Jaeger Authors.
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

package dependencystore_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Opens a badger db and runs a test on it.
func runFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, dr dependencystore.Reader)) {
	f := badger.NewFactory()
	defer func() {
		require.NoError(tb, f.Close())
	}()

	opts := badger.NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v)

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	assert.NoError(tb, err)

	sw, err := f.CreateSpanWriter()
	assert.NoError(tb, err)

	dr, err := f.CreateDependencyReader()
	assert.NoError(tb, err)

	test(tb, sw, dr)
}

func TestDependencyReader(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, dr dependencystore.Reader) {
		tid := time.Now()
		links, err := dr.GetDependencies(context.Background(), tid, time.Hour)
		assert.NoError(t, err)
		assert.Empty(t, links)

		traces := 40
		spans := 3
		for i := 0; i < traces; i++ {
			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(i),
						High: 1,
					},
					SpanID:        model.SpanID(j),
					OperationName: "operation-a",
					Process: &model.Process{
						ServiceName: fmt.Sprintf("service-%d", j),
					},
					StartTime: tid.Add(time.Duration(i)),
					Duration:  time.Duration(i + j),
				}
				if j > 0 {
					s.References = []model.SpanRef{model.NewChildOfRef(s.TraceID, model.SpanID(j-1))}
				}
				err := sw.WriteSpan(context.Background(), &s)
				assert.NoError(t, err)
			}
		}
		links, err = dr.GetDependencies(context.Background(), time.Now(), time.Hour)
		assert.NoError(t, err)
		assert.NotEmpty(t, links)
		assert.Equal(t, spans-1, len(links))                // First span does not create a dependency
		assert.Equal(t, uint64(traces), links[0].CallCount) // Each trace calls the same services
	})
}

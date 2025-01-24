// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencystore_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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

	cfg := badger.DefaultConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v, zap.NewNop())

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(tb, err)

	sw, err := f.CreateSpanWriter()
	require.NoError(tb, err)

	dr, err := f.CreateDependencyReader()
	require.NoError(tb, err)

	test(tb, sw, dr)
}

func TestDependencyReader(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, dr dependencystore.Reader) {
		tid := time.Now()
		links, err := dr.GetDependencies(context.Background(), tid, time.Hour)
		require.NoError(t, err)
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
				require.NoError(t, err)
			}
		}
		links, err = dr.GetDependencies(context.Background(), time.Now(), time.Hour)
		require.NoError(t, err)
		assert.NotEmpty(t, links)
		assert.Len(t, links, spans-1)                       // First span does not create a dependency
		assert.Equal(t, uint64(traces), links[0].CallCount) // Each trace calls the same services
	})
}

// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"testing"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type storageHost struct {
	extension component.Component
}

func (storageHost) ReportFatalError(error) {
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		remotesampling.ID: host.extension,
	}
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

// type fakeRSExtension struct{}

// var _ remotesampling.Extension = (*fakeRSExtension)(nil)

// func (f *fakeRSExtension) Start(ctx context.Context, host component.Host) error {
// 	return nil
// }

// func (f *fakeRSExtension) Shutdown(ctx context.Context) error {
// 	return nil
// }

func TestNewTraceProcessor(t *testing.T) {
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}
	config, ok := createDefaultConfig().(*Config)
	require.True(t, ok)
	newTraceProcessor := newTraceProcessor(*config, telemetrySettings)
	require.NotNil(t, newTraceProcessor)
}

func TestTraceProcessorStart(t *testing.T) {
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}
	config, ok := createDefaultConfig().(*Config)
	require.True(t, ok)
	traceProcessor := newTraceProcessor(*config, telemetrySettings)
	err := traceProcessor.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)
}

// func TestTraceProcessorStarttw(t *testing.T) {
// 	telemetrySettings := component.TelemetrySettings{
// 		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
// 	}
// 	traceProcessor := newTraceProcessor(Config{}, telemetrySettings)
// 	getAdaptiveSamplingComponents := remotesampling.AdaptiveSamplingComponents{
// 		SamplingStore: ,
// 	}
// 	testHost := storageHost{
// 		extension: &remotesampling.RsExtension{
// 			AdaptiveStore: getAdaptiveSamplingComponents.SamplingStore,
// 			DistLock:      getAdaptiveSamplingComponents.DistLock,
// 		},
// 	}

// 	err := traceProcessor.start(context.Background(), testHost)
// 	require.NoError(t, err)
// }

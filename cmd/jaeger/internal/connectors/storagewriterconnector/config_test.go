// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

func TestConfig_Validate(t *testing.T) {
	blocking := exporterhelper.NewDefaultQueueConfig()
	blocking.WaitForResult = true
	acknowledgingOnEnqueue := exporterhelper.NewDefaultQueueConfig()

	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{name: "no queue", mutate: func(*Config) {}},
		{
			name:   "blocking queue",
			mutate: func(cfg *Config) { cfg.QueueConfig = configoptional.Some(blocking) },
		},
		{
			name:    "queue that acknowledges on enqueue",
			mutate:  func(cfg *Config) { cfg.QueueConfig = configoptional.Some(acknowledgingOnEnqueue) },
			wantErr: "queue.wait_for_result must be true",
		},
		{
			name:    "missing trace_storage",
			mutate:  func(cfg *Config) { cfg.TraceStorage = "" },
			wantErr: "TraceStorage: non zero value required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := directConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

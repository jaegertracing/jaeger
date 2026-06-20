// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBuildRotation(t *testing.T) {
	date := time.Date(2019, time.October, 10, 5, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		opts          RotationBuilderOptions
		expectedWrite string
		expectedRead  []string
	}{
		{
			name: "periodic daily",
			opts: RotationBuilderOptions{
				IndexPrefix:       "jaeger-span-",
				DateLayout:        "2006-01-02",
				RolloverFrequency: -24 * time.Hour,
			},
			expectedWrite: "jaeger-span-2019-10-10",
			expectedRead:  []string{"jaeger-span-2019-10-10"},
		},
		{
			name: "periodic hourly",
			opts: RotationBuilderOptions{
				IndexPrefix:       "jaeger-span-",
				DateLayout:        "2006-01-02-15",
				RolloverFrequency: -1 * time.Hour,
			},
			expectedWrite: "jaeger-span-2019-10-10-05",
			expectedRead:  []string{"jaeger-span-2019-10-10-05"},
		},
		{
			name: "alias with default suffixes",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
			},
			expectedWrite: "jaeger-span-write",
			expectedRead:  []string{"jaeger-span-read"},
		},
		{
			name: "alias with custom suffixes",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "archive",
				ReadSuffix:          "archive",
			},
			expectedWrite: "jaeger-span-archive",
			expectedRead:  []string{"jaeger-span-archive"},
		},
		{
			name: "explicit aliases override everything",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
				ExplicitWriteAlias:  "custom-span-write",
				ExplicitReadAlias:   "custom-span-read",
			},
			expectedWrite: "custom-span-write",
			expectedRead:  []string{"custom-span-read"},
		},
		{
			name: "periodic with remote clusters",
			opts: RotationBuilderOptions{
				IndexPrefix:        "jaeger-span-",
				DateLayout:         "2006-01-02",
				RolloverFrequency:  -24 * time.Hour,
				RemoteReadClusters: []string{"cluster_one", "cluster_two"},
			},
			expectedWrite: "jaeger-span-2019-10-10",
			expectedRead: []string{
				"jaeger-span-2019-10-10",
				"cluster_one:jaeger-span-2019-10-10",
				"cluster_two:jaeger-span-2019-10-10",
			},
		},
		{
			name: "alias with remote clusters",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
				RemoteReadClusters:  []string{"cluster_one", "cluster_two"},
			},
			expectedWrite: "jaeger-span-write",
			expectedRead: []string{
				"jaeger-span-read",
				"cluster_one:jaeger-span-read",
				"cluster_two:jaeger-span-read",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := BuildRotation(tt.opts)
			assert.Equal(t, tt.expectedWrite, r.WriteTarget(date))
			assert.Equal(t, tt.expectedRead, r.ReadTargets(date, date))
			assert.Equal(t, WriteOpIndex, r.WriteOpType())
		})
	}
}

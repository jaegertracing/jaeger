// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package lookback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client/mocks"
)

func TestLookBackAction(t *testing.T) {
	nowTime := time.Date(2021, 10, 12, 10, 10, 10, 10, time.Local)
	indices := []client.Index{
		{
			Index: "jaeger-span-archive-0000",
			Aliases: map[string]bool{
				"jaeger-span-archive-other-alias": true,
			},
			CreationTime: time.Date(2021, 10, 10, 10, 10, 10, 10, time.Local),
		},
		{
			Index: "jaeger-span-archive-0001",
			Aliases: map[string]bool{
				"jaeger-span-archive-read": true,
			},
			CreationTime: time.Date(2021, 10, 10, 10, 10, 10, 10, time.Local),
		},
		{
			Index: "jaeger-span-archive-0002",
			Aliases: map[string]bool{
				"jaeger-span-archive-read":  true,
				"jaeger-span-archive-write": true,
			},
			CreationTime: time.Date(2021, 10, 11, 10, 10, 10, 10, time.Local),
		},
		{
			Index: "jaeger-span-archive-0002",
			Aliases: map[string]bool{
				"jaeger-span-archive-read": true,
			},
			CreationTime: nowTime,
		},
		{
			Index: "jaeger-span-archive-0004",
			Aliases: map[string]bool{
				"jaeger-span-archive-read":  true,
				"jaeger-span-archive-write": true,
			},
			CreationTime: nowTime,
		},
	}

	timeNow = func() time.Time {
		return nowTime
	}

	tests := []struct {
		name                  string
		setupCallExpectations func(indexClient *mocks.IndexAPI)
		config                Config
		expectedErr           error
	}{
		{
			name: "success",
			setupCallExpectations: func(indexClient *mocks.IndexAPI) {
				indexClient.On("GetJaegerIndices", "").Return(indices, nil)
				indexClient.On("DeleteAlias", []client.Alias{
					{
						Index: "jaeger-span-archive-0001",
						Name:  "jaeger-span-archive-read",
					},
				}).Return(nil)
			},
			config: Config{
				Unit:      "days",
				UnitCount: 1,
				Config: app.Config{
					Archive: true,
					UseILM:  true,
				},
			},
			expectedErr: nil,
		},
		{
			name: "get indices error",
			setupCallExpectations: func(indexClient *mocks.IndexAPI) {
				indexClient.On("GetJaegerIndices", "").Return(indices, errors.New("get indices error"))
			},
			config: Config{
				Unit:      "days",
				UnitCount: 1,
				Config: app.Config{
					Archive: true,
					UseILM:  true,
				},
			},
			expectedErr: errors.New("get indices error"),
		},
		{
			name: "empty indices",
			setupCallExpectations: func(indexClient *mocks.IndexAPI) {
				indexClient.On("GetJaegerIndices", "").Return([]client.Index{}, nil)
			},
			config: Config{
				Unit:      "days",
				UnitCount: 1,
				Config: app.Config{
					Archive: true,
					UseILM:  true,
				},
			},
			expectedErr: nil,
		},
	}

	logger, _ := zap.NewProduction()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexClient := &mocks.IndexAPI{}
			lookbackAction := Action{
				Config:        test.config,
				IndicesClient: indexClient,
				Logger:        logger,
			}

			test.setupCallExpectations(indexClient)

			err := lookbackAction.Do(context.TODO())
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr, err)
			}
		})
	}
}

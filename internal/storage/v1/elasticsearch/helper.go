// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
)

type mockClientBuilder struct {
	err                 error
	createTemplateError error
}

func (m *mockClientBuilder) NewClient(*escfg.Configuration, *zap.Logger, metrics.Factory) (es.Client, error) {
	if m.err == nil {
		c := &mocks.Client{}
		tService := &mocks.TemplateCreateService{}
		dService := &mocks.IndicesDeleteService{}
		tService.On("Body", mock.Anything).Return(tService)
		tService.On("Do", context.Background()).Return(nil, m.createTemplateError)
		c.On("CreateTemplate", mock.Anything).Return(tService)
		c.On("GetVersion").Return(uint(6))
		c.On("Close").Return(nil)
		c.On("DeleteIndex", mock.Anything).Return(dService)
		dService.On("Do", mock.Anything).Return(nil, nil)
		return c, nil
	}
	return nil, m.err
}

func GetTestingFactoryBase(t *testing.T) *FactoryBase {
	f := NewFactoryBase()
	f.newClientFn = (&mockClientBuilder{}).NewClient
	f.logger = zaptest.NewLogger(t)
	f.metricsFactory = metrics.NullFactory
	f.config = &escfg.Configuration{}
	return f
}

func GetTestingFactoryBaseWithCreateTemplateError(t *testing.T, err error) *FactoryBase {
	f := NewFactoryBase()
	f.newClientFn = (&mockClientBuilder{createTemplateError: err}).NewClient
	f.logger = zaptest.NewLogger(t)
	f.metricsFactory = metrics.NullFactory
	f.config = &escfg.Configuration{}
	return f
}

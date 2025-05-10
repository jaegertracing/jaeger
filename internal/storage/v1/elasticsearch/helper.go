// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

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

func SetFactoryForTest(f *FactoryBase, logger *zap.Logger, metricsFactory metrics.Factory) {
	SetFactoryForTestWithCreateTemplateErr(f, logger, metricsFactory, nil)
}

func SetFactoryForTestWithCreateTemplateErr(f *FactoryBase, logger *zap.Logger, metricsFactory metrics.Factory, err error) {
	f.newClientFn = (&mockClientBuilder{createTemplateError: err}).NewClient
	f.logger = logger
	f.metricsFactory = metricsFactory
}

func SetFactoryForTestWithMappingErr(f *FactoryBase, logger *zap.Logger, metricsFactory metrics.Factory, err error) {
	f.newClientFn = (&mockClientBuilder{}).NewClient
	tmplBuilder := &mocks.TemplateBuilder{}
	tmplBuilder.On("Parse", mock.Anything).Return(nil, err)
	f.templateBuilder = tmplBuilder
	f.logger = logger
	f.metricsFactory = metricsFactory
}

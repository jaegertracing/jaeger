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
//go:build badger_storage_integration
// +build badger_storage_integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	logger  *zap.Logger
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize() error {
	s.factory = badger.NewFactory()

	err := s.factory.Initialize(metrics.NullFactory, zap.NewNop())
	if err != nil {
		return err
	}

	sw, err := s.factory.CreateSpanWriter()
	if err != nil {
		return err
	}
	sr, err := s.factory.CreateSpanReader()
	if err != nil {
		return err
	}
	if s.SamplingStore, err = s.factory.CreateSamplingStore(0); err != nil {
		return err
	}

	s.SpanReader = sr
	s.SpanWriter = sw

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp

	logger, _ := testutils.NewLogger()
	s.logger = logger

	// TODO: remove this badger supports returning spanKind from GetOperations
	s.GetOperationsMissingSpanKind = true
	return nil
}

func (s *BadgerIntegrationStorage) clear() error {
	return s.factory.Close()
}

func (s *BadgerIntegrationStorage) cleanUp() error {
	err := s.clear()
	if err != nil {
		return err
	}
	return s.initialize()
}

func (s *BadgerIntegrationStorage) refresh() error {
	return nil
}

func TestBadgerStorage(t *testing.T) {
	s := &BadgerIntegrationStorage{}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
	defer s.clear()
}

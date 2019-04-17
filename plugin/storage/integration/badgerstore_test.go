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

package integration

import (
	"fmt"
	"io"
	"os"
	"testing"

	assert "github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *BadgerIntegrationStorage) initialize() error {
	f := badger.NewFactory()

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	if err != nil {
		return err
	}

	sw, err := f.CreateSpanWriter()
	if err != nil {
		return err
	}
	sr, err := f.CreateSpanReader()
	if err != nil {
		return err
	}

	s.SpanReader = sr
	s.SpanWriter = sw

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp

	logger, _ := testutils.NewLogger()
	s.logger = logger

	return nil
}

func (s *BadgerIntegrationStorage) clear() error {
	if closer, ok := s.SpanWriter.(io.Closer); ok {
		err := closer.Close()
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("BadgerIntegrationStorage did not implement io.Closer, unable to close and cleanup the storage correctly")
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
	if os.Getenv("STORAGE") != "badger" {
		t.Skip("Integration test against Badger skipped; set STORAGE env var to badger to run this")
	}
	s := &BadgerIntegrationStorage{}
	assert.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
	defer s.clear()
}

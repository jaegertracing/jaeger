// Copyright (c) 2021 The Jaeger Authors.
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

package adaptive

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/hostname"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

// NewStrategyStore creates a strategy store that holds adaptive sampling strategies.
func NewStrategyStore(options Options, metricsFactory metrics.Factory, logger *zap.Logger, lock distributedlock.Lock, store samplingstore.Store) (*Processor, error) {
	hostname, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	logger.Info("Using unique participantName in adaptive sampling", zap.String("participantName", hostname))

	participant := leaderelection.NewElectionParticipant(lock, defaultResourceName, leaderelection.ElectionParticipantOptions{
		FollowerLeaseRefreshInterval: options.FollowerLeaseRefreshInterval,
		LeaderLeaseRefreshInterval:   options.LeaderLeaseRefreshInterval,
		Logger:                       logger,
	})
	p, err := newProcessor(options, hostname, store, participant, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	return p, nil
}

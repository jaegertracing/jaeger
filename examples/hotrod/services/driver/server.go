// Copyright (c) 2017 Uber Technologies, Inc.
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

package driver

import (
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/driver/thrift-gen/driver"
)

const maxGetDriverAttempts = 3

// Server implements jaeger-demo-frontend service
type Server struct {
	hostPort string
	tracer   opentracing.Tracer
	logger   log.Factory
	ch       *tchannel.Channel
	server   *thrift.Server
	redis    *Redis
}

// NewServer creates a new driver.Server
func NewServer(hostPort string, tracer opentracing.Tracer, metricsFactory metrics.Factory, logger log.Factory) *Server {
	channelOpts := &tchannel.ChannelOptions{
		Tracer: tracer,
	}
	ch, err := tchannel.NewChannel("driver", channelOpts)
	if err != nil {
		logger.Bg().Fatal("Cannot create TChannel", zap.Error(err))
	}
	server := thrift.NewServer(ch)

	return &Server{
		hostPort: hostPort,
		tracer:   tracer,
		logger:   logger,
		ch:       ch,
		server:   server,
		redis:    newRedis(metricsFactory, logger),
	}
}

// Run starts the Driver server
func (s *Server) Run() error {

	s.server.Register(driver.NewTChanDriverServer(s))

	if err := s.ch.ListenAndServe(s.hostPort); err != nil {
		s.logger.Bg().Fatal("Unable to start tchannel server", zap.Error(err))
	}

	peerInfo := s.ch.PeerInfo()
	s.logger.Bg().Info("TChannel listening", zap.String("hostPort", peerInfo.HostPort))

	// Run must block, but TChannel's ListenAndServe runs in the background, so block indefinitely
	select {}
}

// FindNearest implements Thrift interface TChanDriver. config.GetDriversConcurrently adds concurrency.
func (s *Server) FindNearest(ctx thrift.Context, location string) ([]*driver.DriverLocation, error) {
	s.logger.For(ctx).Info("Searching for nearby drivers", zap.String("location", location))
	driverIDs := s.redis.FindDriverIDs(ctx, location)
	retMe := make([]*driver.DriverLocation, len(driverIDs))
	var topErr error
	wg := sync.WaitGroup{}
	rvLock := sync.Mutex{}
	// Sequester getting a driver into a function so can run concurrently
	getDriver := func(driverID string, driverIdx int) {
		defer wg.Done()
		var drvr Driver
		var err error
		for i := 0; i < maxGetDriverAttempts; i++ {
			if err != nil {
				s.logger.For(ctx).Error("Retrying GetDriver after error", zap.Int("retry_no", i+1), zap.Error(err))
			}
			drvr, err = s.redis.GetDriver(ctx, driverID)
			if err == nil {
				break
			}
		}
		rvLock.Lock()
		defer rvLock.Unlock()
		if err != nil {
			s.logger.For(ctx).Error("Failed to get driver", zap.Error(err), zap.Int("num_attempts", maxGetDriverAttempts))
			topErr = err
		} else {
			retMe[driverIdx] = &driver.DriverLocation{
				DriverID: drvr.DriverID,
				Location: drvr.Location,
			}
		}
	}
	for i, driverID := range driverIDs {
		if config.GetDriversConcurrently {
			wg.Add(1)
			go getDriver(driverID, i)
		} else if topErr == nil {
			wg.Add(1)
			getDriver(driverID, i)
		} else {
			// Have a topErr, so bail early
			break
		}
	}
	wg.Wait()
	if topErr != nil {
		s.logger.For(ctx).Error("Search failed to get the driver at least one time")
		return nil, topErr
	}
	s.logger.For(ctx).Info("Search successful", zap.Int("num_drivers", len(retMe)))
	return retMe, nil
}

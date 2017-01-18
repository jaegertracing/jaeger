package driver

import (
	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/services/driver/thrift-gen/driver"
)

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
func NewServer(hostPort string, tracer opentracing.Tracer, logger log.Factory) *Server {
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
		redis:    newRedis(logger),
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

	// Run must block, but TChannel's ListenAndServer runs in the background, so block indefinitely
	select {}
}

// FindNearest implements Thrift interface TChanDriver
func (s *Server) FindNearest(ctx thrift.Context, location string) ([]*driver.DriverLocation, error) {
	s.logger.For(ctx).Info("Searching for nearby drivers", zap.String("location", location))
	driverIDs := s.redis.FindDriverIDs(ctx, location)

	retMe := make([]*driver.DriverLocation, len(driverIDs))
	for i, driverID := range driverIDs {
		var drv Driver
		var err error
		for i := 0; i < 3; i++ {
			drv, err = s.redis.GetDriver(ctx, driverID)
			if err == nil {
				break
			}
			s.logger.For(ctx).Error("Retrying GetDriver after error", zap.Int("retry_no", i+1), zap.Error(err))
		}
		if err != nil {
			s.logger.For(ctx).Error("Failed to get driver after 3 attempts", zap.Error(err))
			return nil, err
		}
		retMe[i] = &driver.DriverLocation{
			DriverID: drv.DriverID,
			Location: drv.Location,
		}
	}
	s.logger.For(ctx).Info("Search successful", zap.Int("num_drivers", len(retMe)))
	return retMe, nil
}

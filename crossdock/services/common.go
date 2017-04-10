package services

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"code.uber.internal/go-common.git/x/tchannel"

	"golang.org/x/net/context"
)

func healthCheck(logger *zap.Logger, service, healthURL string) {
	for i := 0; i < 100; i++ {
		_, err := http.Get(healthURL)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}

func tChannelHealthCheck(logger *zap.Logger, service, hostPort string) {
	cfg := xtchannel.Configuration{DisableHyperbahn: true}
	channel, err := cfg.NewClient("test_driver", nil)
	if err != nil {
		logger.Fatal("Could not initialize tchannel for health check")
	}
	for i := 0; i < 100; i++ {
		err := channel.Ping(context.Background(), hostPort)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}

func getTracerServiceName(service string) string {
	return fmt.Sprintf("crossdock-%s", service)
}

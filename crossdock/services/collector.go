package services

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

const (
	collectorService = "Collector"

	collectorCmd      = "/go/cmd/jaeger-collector -f %s &"
	collectorHostPort = "localhost:4267"
)

// InitializeCollector initializes the jaeger collector
func InitializeCollector(logger *zap.Logger) {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf(collectorCmd, collectorConfig))
	err := cmd.Run()
	if err != nil {
		logger.Fatal("Failed to initialize collector service", zap.Error(err))
	}
	tChannelHealthCheck(logger, collectorService, collectorHostPort)
}

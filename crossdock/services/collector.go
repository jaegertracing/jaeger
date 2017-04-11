package services

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

const (
	collectorService = "Collector"

	collectorCmd      = "/go/cmd/jaeger-collector %s &"
	collectorHostPort = "localhost:14267"
)

// InitializeCollector initializes the jaeger collector
func InitializeCollector(logger *zap.Logger) {
	cmd := exec.Command("/bin/bash", "-c",
		fmt.Sprintf(collectorCmd, "-cassandra.keyspace=jaeger -cassandra.servers=cassandra -cassandra.connections-per-host=1"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize collector service", zap.Error(err))
	}
	tChannelHealthCheck(logger, collectorService, collectorHostPort)
}

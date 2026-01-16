// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckv2extension"
)

var (
	_ component.Host           = (*HealthCheckHost)(nil)
	_ componentstatus.Reporter = (*HealthCheckHost)(nil)
)

// statusWatcherExtension is an extension that can receive component status changes.
type statusWatcherExtension interface {
	extension.Extension
	ComponentStatusChanged(source *componentstatus.InstanceID, event *componentstatus.Event)
}

// HealthCheckHost is a component.Host implementation that wraps the OTEL
// healthcheckv2 extension for standalone (non-Collector) binaries.
// It implements componentstatus.Reporter to receive status updates.
type HealthCheckHost struct {
	extension  statusWatcherExtension
	instanceID *componentstatus.InstanceID
}

// NewHealthCheckHost creates a HealthCheckHost that runs the healthcheckv2
// extension on the specified HTTP endpoint.
func NewHealthCheckHost(ctx context.Context, telset component.TelemetrySettings, endpoint string) (*HealthCheckHost, error) {
	factory := healthcheckv2extension.NewFactory()
	cfg := factory.CreateDefaultConfig().(*healthcheckv2extension.Config)

	// Configure HTTP endpoint (legacy mode for simple / health endpoint)
	cfg.LegacyConfig.ServerConfig = confighttp.ServerConfig{
		Endpoint: endpoint,
	}
	cfg.UseV2 = false // Use legacy mode for simple health endpoint

	extSettings := extension.Settings{
		ID:                component.MustNewID("healthcheck"),
		TelemetrySettings: telset,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}

	ext, err := factory.Create(ctx, extSettings, cfg)
	if err != nil {
		return nil, err
	}

	// Create an InstanceID for this standalone component
	instanceID := componentstatus.NewInstanceID(
		component.MustNewID("jaeger"),
		component.KindExtension,
	)

	return &HealthCheckHost{
		extension:  ext.(statusWatcherExtension),
		instanceID: instanceID,
	}, nil
}

// Start starts the healthcheck extension's HTTP server.
func (h *HealthCheckHost) Start(ctx context.Context) error {
	return h.extension.Start(ctx, h)
}

// Shutdown stops the healthcheck extension.
func (h *HealthCheckHost) Shutdown(ctx context.Context) error {
	return h.extension.Shutdown(ctx)
}

// GetExtensions implements component.Host.
func (h *HealthCheckHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		component.MustNewID("healthcheck"): h.extension,
	}
}

// Report implements componentstatus.Reporter.
// It forwards status events to the healthcheckv2 extension.
func (h *HealthCheckHost) Report(event *componentstatus.Event) {
	h.extension.ComponentStatusChanged(h.instanceID, event)
}

// Ready reports that the service is ready to handle requests.
func (h *HealthCheckHost) Ready() {
	h.Report(componentstatus.NewEvent(componentstatus.StatusOK))
}

// SetUnavailable reports that the service is not available.
func (h *HealthCheckHost) SetUnavailable() {
	h.Report(componentstatus.NewEvent(componentstatus.StatusStopping))
}

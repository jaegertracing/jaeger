// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expvar

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

var _ extension.Extension = (*expvarExtension)(nil)

const (
	Port = 27777
)

type expvarExtension struct {
	config *Config
	server *http.Server
	telset component.TelemetrySettings
}

func newExtension(config *Config, telset component.TelemetrySettings) *expvarExtension {
	return &expvarExtension{
		config: config,
		telset: telset,
	}
}

func (c *expvarExtension) Start(_ context.Context, host component.Host) error {
	c.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", c.config.Port),
		Handler:           expvar.Handler(),
		ReadHeaderTimeout: 3 * time.Second,
	}
	c.telset.Logger.Info("Starting expvar server", zap.String("addr", c.server.Addr))
	go func() {
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("error starting expvar server: %w", err)
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

func (c *expvarExtension) Shutdown(ctx context.Context) error {
	if c.server != nil {
		if err := c.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down expvar server: %w", err)
		}
	}
	return nil
}

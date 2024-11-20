// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expvar

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"sync"

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

	shutdownWG sync.WaitGroup
}

func newExtension(config *Config, telset component.TelemetrySettings) *expvarExtension {
	return &expvarExtension{
		config: config,
		telset: telset,
	}
}

func (c *expvarExtension) Start(ctx context.Context, host component.Host) error {
	server, err := c.config.ToServer(ctx, host, c.telset, expvar.Handler())
	if err != nil {
		return err
	}
	c.server = server
	c.telset.Logger.Info("Starting expvar server", zap.String("addr", c.server.Addr))

	var hln net.Listener
	if hln, err = c.config.ToListener(ctx); err != nil {
		return err
	}

	c.shutdownWG.Add(1)
	go func() {
		defer c.shutdownWG.Done()

		if err := c.server.Serve(hln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
		c.shutdownWG.Wait()
	}
	return nil
}

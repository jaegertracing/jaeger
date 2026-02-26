// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
)

var (
	_ extension.Extension             = (*storageCleaner)(nil)
	_ extensioncapabilities.Dependent = (*storageCleaner)(nil)
)

const (
	Port = "9231"
	URL  = "/purge"
)

type storageCleaner struct {
	config *Config
	server *http.Server
	telset component.TelemetrySettings
}

func newStorageCleaner(config *Config, telset component.TelemetrySettings) *storageCleaner {
	return &storageCleaner{
		config: config,
		telset: telset,
	}
}

func (c *storageCleaner) Start(_ context.Context, host component.Host) error {
	purger, err := jaegerstorage.GetPurger(c.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("failed to obtain purger: %w", err)
	}

	purgeHandler := func(w http.ResponseWriter, r *http.Request) {
		if err := purger.Purge(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Purge request processed successfully"))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+URL, purgeHandler)
	c.server = &http.Server{
		Addr:              ":" + c.config.Port,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	c.telset.Logger.Info("Starting storage cleaner server", zap.String("addr", c.server.Addr))
	go func() {
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("error starting cleaner server: %w", err)
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

func (c *storageCleaner) Shutdown(ctx context.Context) error {
	if c.server != nil {
		if err := c.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down cleaner server: %w", err)
		}
	}
	return nil
}

func (*storageCleaner) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

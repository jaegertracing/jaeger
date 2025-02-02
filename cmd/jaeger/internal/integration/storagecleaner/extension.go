// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	storage "github.com/jaegertracing/jaeger/internal/storage/v1/api"
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
	storageFactory, err := jaegerstorage.GetStorageFactory(c.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory '%s': %w", c.config.TraceStorage, err)
	}

	purgeStorage := func(httpContext context.Context) error {
		purger, ok := storageFactory.(storage.Purger)
		if !ok {
			return fmt.Errorf("storage %s does not implement Purger interface", c.config.TraceStorage)
		}
		if err := purger.Purge(httpContext); err != nil {
			return fmt.Errorf("error purging storage: %w", err)
		}
		return nil
	}

	purgeHandler := func(w http.ResponseWriter, r *http.Request) {
		if err := purgeStorage(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Purge request processed successfully"))
	}

	r := mux.NewRouter()
	r.HandleFunc(URL, purgeHandler).Methods(http.MethodPost)
	c.server = &http.Server{
		Addr:              ":" + c.config.Port,
		Handler:           r,
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

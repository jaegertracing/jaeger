// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badgercleaner

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

var (
	_ extension.Extension = (*cleaner)(nil)
	_ extension.Dependent = (*cleaner)(nil)
)

const (
	cleanerPort = "9231"
	cleanerURL  = "/purge"
)

type cleaner struct {
	config *Config
	server *http.Server
}

func newBadgerCleaner(config *Config) *cleaner {
	return &cleaner{
		config: config,
	}
}

func (c *cleaner) Start(ctx context.Context, host component.Host) error {
	storageFactory, err := jaegerstorage.GetStorageFactory(c.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory for Badger: %w", err)
	}

	purgeBadger := func() error {
		badgerFactory := storageFactory.(*badger.Factory)
		if err := badgerFactory.Purge(); err != nil {
			return fmt.Errorf("error purging Badger storage: %w", err)
		}
		return nil
	}

	cleanerHandler := func(w http.ResponseWriter, r *http.Request) {
		if err := purgeBadger(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Purge request processed successfully"))
	}

	r := mux.NewRouter()
	r.HandleFunc(cleanerURL, cleanerHandler).Methods(http.MethodPost)
	if c.config.cleanerPort == "" {
		c.config.cleanerPort = cleanerPort
	}
	c.server = &http.Server{
		Addr:              ":" + c.config.cleanerPort,
		Handler:           r,
		ReadHeaderTimeout: 3 * time.Second,
	}
	go func() {
		if err := c.server.ListenAndServe(); err != nil {
			fmt.Printf("Error starting cleaner server: %v\n", err)
		}
	}()

	return nil
}

func (c *cleaner) Shutdown(ctx context.Context) error {
	if c.server != nil {
		err := c.server.Shutdown(ctx)
		if err != nil {
			return fmt.Errorf("error shutting down cleaner server: %w", err)
		}
	}
	return nil
}

func (c *cleaner) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

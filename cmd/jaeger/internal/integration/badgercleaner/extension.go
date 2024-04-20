package badgercleaner

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
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
}

func newBadgerCleaner(config *Config) *cleaner {
	return &cleaner{
		config: config,
	}
}

func (e *cleaner) Start(ctx context.Context, host component.Host) error {
	storageFactory, err := jaegerstorage.GetStorageFactory(e.config.TraceStorage, host)
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
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Purge request processed successfully"))
	}

	r := mux.NewRouter()
	r.HandleFunc(cleanerURL, cleanerHandler).Methods(http.MethodPost)
	go func() {
		if err := http.ListenAndServe(":"+cleanerPort, r); err != nil {
			fmt.Printf("Error starting cleaner server: %v\n", err)
		}
	}()

	return nil
}

func (r *cleaner) Shutdown(ctx context.Context) error {
	return nil
}

func (s *cleaner) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

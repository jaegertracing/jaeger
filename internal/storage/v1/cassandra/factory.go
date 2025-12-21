// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"
	"errors"
	"io"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/hostname"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	gocqlw "github.com/jaegertracing/jaeger/internal/storage/cassandra/gocql"
	caslock "github.com/jaegertracing/jaeger/internal/storage/distributedlock/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	cdepstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/dependencystore"
	csamplingstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/schema"
	cspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
)

var ( // interface comformance checks
	_ storage.Purger               = (*Factory)(nil)
	_ storage.SamplingStoreFactory = (*Factory)(nil)
	_ io.Closer                    = (*Factory)(nil)
	_ storage.ArchiveCapable       = (*Factory)(nil)
)

// Factory for Cassandra backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	config config.Configuration

	session cassandra.Session

	// tests can override this
	sessionBuilderFn func(*config.Configuration) (cassandra.Session, error)
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		tracer:           otel.GetTracerProvider(),
		Options:          NewOptions(),
		sessionBuilderFn: NewSession,
	}
}

// InitFromOptions initializes factory from options.
func (f *Factory) ConfigureFromOptions(o *Options) {
	f.Options = o
	f.config = o.GetConfig()
}

// Initialize performs internal initialization of the factory.
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory = metricsFactory
	f.logger = logger

	session, err := f.sessionBuilderFn(&f.config)
	if err != nil {
		return err
	}
	f.session = session

	return nil
}

// createSession creates session from a configuration
func createSession(c *config.Configuration) (cassandra.Session, error) {
	cluster, err := c.NewCluster()
	if err != nil {
		return nil, err
	}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}

	return gocqlw.WrapCQLSession(session), nil
}

// newSessionPrerequisites creates tables and types before creating a session
func newSessionPrerequisites(c *config.Configuration) error {
	if !c.Schema.CreateSchema {
		return nil
	}

	cfg := *c // clone because we need to connect without specifying a keyspace
	cfg.Schema.Keyspace = ""

	session, err := createSession(&cfg)
	if err != nil {
		return err
	}

	sc := schema.NewSchemaCreator(session, c.Schema)
	return sc.CreateSchemaIfNotPresent()
}

// NewSession creates a new Cassandra session
func NewSession(c *config.Configuration) (cassandra.Session, error) {
	if err := newSessionPrerequisites(c); err != nil {
		return nil, err
	}

	return createSession(c)
}

// CreateSpanReader implements storage.Factory
func (*Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, errors.New("not implemented")
}

// CreateSpanWriter creates a spanstore.Writer.
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	options, err := writerOptions(f.Options)
	if err != nil {
		return nil, err
	}
	return cspanstore.NewSpanWriter(f.session, f.Options.SpanStoreWriteCacheTTL, f.metricsFactory, f.logger, options...)
}

// CreateDependencyReader creates a dependencystore.Reader.
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	version := cdepstore.GetDependencyVersion(f.session)
	return cdepstore.NewDependencyStore(f.session, f.metricsFactory, f.logger, version)
}

// CreateLock implements storage.SamplingStoreFactory
func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	hostId, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	f.logger.Info("Using unique participantName in the distributed lock", zap.String("participantName", hostId))

	return caslock.NewLock(f.session, hostId), nil
}

// CreateSamplingStore implements storage.SamplingStoreFactory
func (f *Factory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	samplingMetricsFactory := f.metricsFactory.Namespace(
		metrics.NSOptions{
			Tags: map[string]string{
				"role": "sampling",
			},
		},
	)
	return csamplingstore.New(f.session, samplingMetricsFactory, f.logger), nil
}

func writerOptions(opts *Options) ([]cspanstore.Option, error) {
	var tagFilters []dbmodel.TagFilter

	// drop all tag filters
	if !opts.Index.Tags || !opts.Index.ProcessTags || !opts.Index.Logs {
		tagFilters = append(tagFilters, dbmodel.NewTagFilterDropAll(!opts.Index.Tags, !opts.Index.ProcessTags, !opts.Index.Logs))
	}

	// black/white list tag filters
	tagIndexBlacklist := opts.TagIndexBlacklist()
	tagIndexWhitelist := opts.TagIndexWhitelist()
	if len(tagIndexBlacklist) > 0 && len(tagIndexWhitelist) > 0 {
		return nil, errors.New("only one of TagIndexBlacklist and TagIndexWhitelist can be specified")
	}
	if len(tagIndexBlacklist) > 0 {
		tagFilters = append(tagFilters, dbmodel.NewBlacklistFilter(tagIndexBlacklist))
	} else if len(tagIndexWhitelist) > 0 {
		tagFilters = append(tagFilters, dbmodel.NewWhitelistFilter(tagIndexWhitelist))
	}

	if len(tagFilters) == 0 {
		return nil, nil
	} else if len(tagFilters) == 1 {
		return []cspanstore.Option{cspanstore.TagFilter(tagFilters[0])}, nil
	}

	return []cspanstore.Option{cspanstore.TagFilter(dbmodel.NewChainedTagFilter(tagFilters...))}, nil
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	if f.session != nil {
		f.session.Close()
	}

	return nil
}

func (f *Factory) Purge(_ context.Context) error {
	return f.session.Query("TRUNCATE traces").Exec()
}

func (f *Factory) IsArchiveCapable() bool {
	return f.Options.ArchiveEnabled
}

func (f *Factory) GetSession() cassandra.Session {
	return f.session
}

func (f *Factory) GetTracer() trace.TracerProvider {
	return f.tracer
}

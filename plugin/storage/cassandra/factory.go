// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/cassandra/config"
	gocqlw "github.com/jaegertracing/jaeger/pkg/cassandra/gocql"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/hostname"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	cLock "github.com/jaegertracing/jaeger/plugin/pkg/distributedlock/cassandra"
	cDepStore "github.com/jaegertracing/jaeger/plugin/storage/cassandra/dependencystore"
	cSamplingStore "github.com/jaegertracing/jaeger/plugin/storage/cassandra/samplingstore"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/schema"
	cSpanStore "github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	primaryStorageConfig = "cassandra"
	archiveStorageConfig = "cassandra-archive"
)

var ( // interface comformance checks
	_ storage.Factory              = (*Factory)(nil)
	_ storage.Purger               = (*Factory)(nil)
	_ storage.ArchiveFactory       = (*Factory)(nil)
	_ storage.SamplingStoreFactory = (*Factory)(nil)
	_ io.Closer                    = (*Factory)(nil)
	_ plugin.Configurable          = (*Factory)(nil)
)

// Factory implements storage.Factory for Cassandra backend.
type Factory struct {
	Options *Options

	primaryMetricsFactory metrics.Factory
	archiveMetricsFactory metrics.Factory
	logger                *zap.Logger
	tracer                trace.TracerProvider

	primaryConfig config.Configuration
	archiveConfig *config.Configuration

	primarySession cassandra.Session
	archiveSession cassandra.Session

	// tests can override this
	sessionBuilderFn func(*config.Configuration) (cassandra.Session, error)
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		tracer:           otel.GetTracerProvider(),
		Options:          NewOptions(primaryStorageConfig, archiveStorageConfig),
		sessionBuilderFn: NewSession,
	}
}

// NewFactoryWithConfig initializes factory with Config.
func NewFactoryWithConfig(
	opts Options,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	f := NewFactory()
	// use this to help with testing
	b := &withConfigBuilder{
		f:              f,
		opts:           &opts,
		metricsFactory: metricsFactory,
		logger:         logger,
		initializer:    f.Initialize, // this can be mocked in tests
	}
	return b.build()
}

type withConfigBuilder struct {
	f              *Factory
	opts           *Options
	metricsFactory metrics.Factory
	logger         *zap.Logger
	initializer    func(metricsFactory metrics.Factory, logger *zap.Logger) error
}

func (b *withConfigBuilder) build() (*Factory, error) {
	b.f.configureFromOptions(b.opts)
	if err := b.opts.Primary.Validate(); err != nil {
		return nil, err
	}
	err := b.initializer(b.metricsFactory, b.logger)
	if err != nil {
		return nil, err
	}
	return b.f, nil
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.Options.InitFromViper(v)
	f.configureFromOptions(f.Options)
}

// InitFromOptions initializes factory from options.
func (f *Factory) configureFromOptions(o *Options) {
	f.Options = o
	// TODO this is a hack because we do not define defaults in Options
	if o.others == nil {
		o.others = make(map[string]*NamespaceConfig)
	}
	f.primaryConfig = o.GetPrimary()
	f.archiveConfig = f.Options.Get(archiveStorageConfig)
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.primaryMetricsFactory = metricsFactory.Namespace(metrics.NSOptions{Name: "cassandra", Tags: nil})
	f.archiveMetricsFactory = metricsFactory.Namespace(metrics.NSOptions{Name: "cassandra-archive", Tags: nil})
	f.logger = logger

	primarySession, err := f.sessionBuilderFn(&f.primaryConfig)
	if err != nil {
		return err
	}
	f.primarySession = primarySession

	if f.archiveConfig != nil {
		archiveSession, err := f.sessionBuilderFn(f.archiveConfig)
		if err != nil {
			return err
		}
		f.archiveSession = archiveSession
	} else {
		logger.Info("Cassandra archive storage configuration is empty, skipping")
	}
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
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return cSpanStore.NewSpanReader(f.primarySession, f.primaryMetricsFactory, f.logger, f.tracer.Tracer("cSpanStore.SpanReader"))
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	options, err := writerOptions(f.Options)
	if err != nil {
		return nil, err
	}
	return cSpanStore.NewSpanWriter(f.primarySession, f.Options.SpanStoreWriteCacheTTL, f.primaryMetricsFactory, f.logger, options...)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	version := cDepStore.GetDependencyVersion(f.primarySession)
	return cDepStore.NewDependencyStore(f.primarySession, f.primaryMetricsFactory, f.logger, version)
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if f.archiveSession == nil {
		return nil, storage.ErrArchiveStorageNotConfigured
	}
	return cSpanStore.NewSpanReader(f.archiveSession, f.archiveMetricsFactory, f.logger, f.tracer.Tracer("cSpanStore.SpanReader"))
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if f.archiveSession == nil {
		return nil, storage.ErrArchiveStorageNotConfigured
	}
	options, err := writerOptions(f.Options)
	if err != nil {
		return nil, err
	}
	return cSpanStore.NewSpanWriter(f.archiveSession, f.Options.SpanStoreWriteCacheTTL, f.archiveMetricsFactory, f.logger, options...)
}

// CreateLock implements storage.SamplingStoreFactory
func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	hostId, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	f.logger.Info("Using unique participantName in the distributed lock", zap.String("participantName", hostId))

	return cLock.NewLock(f.primarySession, hostId), nil
}

// CreateSamplingStore implements storage.SamplingStoreFactory
func (f *Factory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	return cSamplingStore.New(f.primarySession, f.primaryMetricsFactory, f.logger), nil
}

func writerOptions(opts *Options) ([]cSpanStore.Option, error) {
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
		return []cSpanStore.Option{cSpanStore.TagFilter(tagFilters[0])}, nil
	}

	return []cSpanStore.Option{cSpanStore.TagFilter(dbmodel.NewChainedTagFilter(tagFilters...))}, nil
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	if f.primarySession != nil {
		f.primarySession.Close()
	}
	if f.archiveSession != nil {
		f.archiveSession.Close()
	}

	return nil
}

func (f *Factory) Purge(_ context.Context) error {
	return f.primarySession.Query("TRUNCATE traces").Exec()
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/fswatcher"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
	essamplestore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore"
	esspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	esdepstorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
)

var _ io.Closer = (*FactoryBase)(nil)

// FactoryBase for Elasticsearch backend.
type FactoryBase struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(ctx context.Context, c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory, httpAuth extensionauth.HTTPClient) (es.Client, error)

	config *config.Configuration

	client atomic.Pointer[es.Client]

	pwdFileWatcher *fswatcher.FSWatcher

	templateBuilder es.TemplateBuilder

	tags []string

	authenticator extensionauth.HTTPClient
}

type scriptedMetric struct {
	InitScript    string `json:"init_script"`
	MapScript     string `json:"map_script"`
	CombineScript string `json:"combine_script"`
	ReduceScript  string `json:"reduce_script"`
}

func NewFactoryBase(
	ctx context.Context,
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	httpAuth extensionauth.HTTPClient,
) (*FactoryBase, error) {
	f := &FactoryBase{
		config:        &cfg,
		newClientFn:   config.NewClient,
		tracer:        otel.GetTracerProvider(),
		authenticator: httpAuth,
	}
	f.metricsFactory = metricsFactory
	f.logger = logger
	f.templateBuilder = es.TextTemplateBuilder{}
	tags, err := f.config.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	f.tags = tags

	client, err := f.newClientFn(ctx, f.config, logger, metricsFactory, httpAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	f.client.Store(&client)

	if f.config.Authentication.BasicAuthentication.HasValue() {
		if file := f.config.Authentication.BasicAuthentication.Get().PasswordFilePath; file != "" {
			watcher, err := fswatcher.New([]string{file}, f.onPasswordChange, f.logger)
			if err != nil {
				return nil, fmt.Errorf("failed to create watcher for ES client's password: %w", err)
			}
			f.pwdFileWatcher = watcher
		}
	}

	err = f.createTemplates(ctx)
	if err != nil {
		return nil, err
	}

	err = f.createTraceSummaryTransform(ctx)
	if err != nil {
		f.logger.Warn(
			"Failed to provision trace summary transform; optimization will fall back to global search until transform is available",
			zap.Error(err),
		)
	}

	return f, nil
}

func (f *FactoryBase) getClient() es.Client {
	if c := f.client.Load(); c != nil {
		return *c
	}
	return nil
}

// GetSpanReaderParams returns the SpanReaderParams which can be used to initialize the v1 and v2 readers.
func (f *FactoryBase) GetSpanReaderParams() esspanstore.SpanReaderParams {
	return esspanstore.SpanReaderParams{
		Client:              f.getClient,
		MaxDocCount:         f.config.MaxDocCount,
		MaxSpanAge:          f.config.MaxSpanAge,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		SpanIndex:           f.config.Indices.Spans,
		ServiceIndex:        f.config.Indices.Services,
		TagDotReplacement:   f.config.Tags.DotReplacement,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
		ReadAliasSuffix:     f.config.ReadAliasSuffix,
		RemoteReadClusters:  f.config.RemoteReadClusters,
		SpanReadAlias:       f.config.SpanReadAlias,
		ServiceReadAlias:    f.config.ServiceReadAlias,
		Logger:              f.logger,
		Tracer:              f.tracer.Tracer("esspanstore.SpanReader"),
		UseTraceSummary:     f.config.UseTraceSummary,
		TraceSummaryIndex:   f.config.TraceSummaryIndex,
	}
}

// GetSpanWriterParams returns the SpanWriterParams which can be used to initialize the v1 and v2 writers.
func (f *FactoryBase) GetSpanWriterParams() esspanstore.SpanWriterParams {
	return esspanstore.SpanWriterParams{
		Client:              f.getClient,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		SpanIndex:           f.config.Indices.Spans,
		ServiceIndex:        f.config.Indices.Services,
		AllTagsAsFields:     f.config.Tags.AllAsFields,
		TagKeysAsFields:     f.tags,
		TagDotReplacement:   f.config.Tags.DotReplacement,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
		WriteAliasSuffix:    f.config.WriteAliasSuffix,
		SpanWriteAlias:      f.config.SpanWriteAlias,
		ServiceWriteAlias:   f.config.ServiceWriteAlias,
		Logger:              f.logger,
		MetricsFactory:      f.metricsFactory,
		ServiceCacheTTL:     f.config.ServiceCacheTTL,
	}
}

// GetDependencyStoreParams returns the esdepstorev2.Params which can be used to initialize the v1 and v2 dependency stores.
func (f *FactoryBase) GetDependencyStoreParams() esdepstorev2.Params {
	return esdepstorev2.Params{
		Client:              f.getClient,
		Logger:              f.logger,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		IndexDateLayout:     f.config.Indices.Dependencies.DateLayout,
		MaxDocCount:         f.config.MaxDocCount,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
	}
}

func (f *FactoryBase) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	params := essamplestore.Params{
		Client:                 f.getClient,
		Logger:                 f.logger,
		IndexPrefix:            f.config.Indices.IndexPrefix,
		IndexDateLayout:        f.config.Indices.Sampling.DateLayout,
		IndexRolloverFrequency: config.RolloverFrequencyAsNegativeDuration(f.config.Indices.Sampling.RolloverFrequency),
		Lookback:               f.config.AdaptiveSamplingLookback,
		MaxDocCount:            f.config.MaxDocCount,
	}
	store := essamplestore.NewSamplingStore(params)

	if f.config.CreateIndexTemplates {
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
		samplingMapping, err := mappingBuilder.GetSamplingMappings()
		if err != nil {
			return nil, err
		}
		if _, err := f.getClient().CreateTemplate(params.PrefixedIndexName()).Body(samplingMapping).Do(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to create template: %w", err)
		}
	}

	return store, nil
}

func (f *FactoryBase) mappingBuilderFromConfig(cfg *config.Configuration) mappings.MappingBuilder {
	return mappings.MappingBuilder{
		TemplateBuilder: f.templateBuilder,
		Indices:         cfg.Indices,
		EsVersion:       cfg.Version,
		UseILM:          cfg.UseILM,
	}
}

// Close closes the resources held by the factory
func (f *FactoryBase) Close() error {
	var errs []error

	if f.pwdFileWatcher != nil {
		errs = append(errs, f.pwdFileWatcher.Close())
	}
	errs = append(errs, f.getClient().Close())

	return errors.Join(errs...)
}

func (f *FactoryBase) onPasswordChange() {
	f.onClientPasswordChange(f.config, &f.client, f.metricsFactory)
}

func (f *FactoryBase) onClientPasswordChange(cfg *config.Configuration, client *atomic.Pointer[es.Client], mf metrics.Factory) {
	basicAuth := cfg.Authentication.BasicAuthentication.Get()
	newPassword, err := loadTokenFromFile(basicAuth.PasswordFilePath)
	if err != nil {
		f.logger.Error("failed to reload password for Elasticsearch client", zap.Error(err))
		return
	}
	f.logger.Sugar().Infof("loaded new password of length %d from file", len(newPassword))
	newCfg := *cfg // copy by value
	newCfg.Authentication.BasicAuthentication = configoptional.Some(config.BasicAuthentication{
		Username:         basicAuth.Username,
		Password:         newPassword,
		PasswordFilePath: "", // avoid error that both are set
	})

	newClient, err := f.newClientFn(context.Background(), &newCfg, f.logger, mf, nil)
	if err != nil {
		f.logger.Error("failed to recreate Elasticsearch client with new password", zap.Error(err))
		return
	}
	if oldClient := *client.Swap(&newClient); oldClient != nil {
		if err := oldClient.Close(); err != nil {
			f.logger.Error("failed to close Elasticsearch client", zap.Error(err))
		}
	}
}

func (f *FactoryBase) Purge(ctx context.Context) error {
	esClient := f.getClient()
	_, err := esClient.DeleteIndex("*").Do(ctx)
	return err
}

func loadTokenFromFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func (f *FactoryBase) createTemplates(ctx context.Context) error {
	if f.config.CreateIndexTemplates {
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return err
		}
		jaegerSpanIdx := f.config.Indices.IndexPrefix.Apply("jaeger-span")
		jaegerServiceIdx := f.config.Indices.IndexPrefix.Apply("jaeger-service")
		_, err = f.getClient().CreateTemplate(jaegerSpanIdx).Body(spanMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
		}
		_, err = f.getClient().CreateTemplate(jaegerServiceIdx).Body(serviceMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerServiceIdx, err)
		}
	}
	return nil
}

func (f *FactoryBase) createTraceSummaryTransform(ctx context.Context) error {
	if !f.config.UseTraceSummary {
		return nil
	}

	traceSummaryVersion := f.config.TraceSummaryVersion
	if traceSummaryVersion == "" {
		traceSummaryVersion= "v1"
	}
	jaegerSpanIdx := f.config.Indices.IndexPrefix.Apply("jaeger-span")
	summaryIndex := f.config.TraceSummaryIndex
	if summaryIndex == "" {
		cleanPrefix := strings.TrimSuffix(jaegerSpanIdx, "-")
		if strings.HasSuffix(cleanPrefix, "jaeger-span") {
			summaryIndex = strings.TrimSuffix(cleanPrefix, "jaeger-span") + "trace-summary"
		} else {
			summaryIndex = strings.Replace(cleanPrefix, "jaeger-span", "trace-summary", 1)
		}
	}

	transformID := fmt.Sprintf("%s_%s", summaryIndex, "jaeger_trace_summary_job")

	transport, err := config.GetHTTPRoundTripper(ctx, f.config, f.logger, f.authenticator)
	if err != nil {
		return fmt.Errorf("failed to create HTTP transport: %w", err)
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	esURL, err := f.selectElasticsearchServer(ctx, f.config.Servers, client)
	if err != nil {
		return err
	}

	shouldCreate := true
	err = func() error {
		getURL := fmt.Sprintf("%s/_transform/%s", esURL, transformID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			f.logger.Debug("Transform check failed (network), assuming non-existent", zap.Error(err))
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read transform check response: %w", err)
			}

			var existingConfig struct {
				Transforms []struct {
					Description string `json:"description"`
					Dest        struct {
						Index string `json:"index"`
					} `json:"dest"`
				} `json:"transforms"`
			}
			if err := json.Unmarshal(bodyBytes, &existingConfig); err == nil && len(existingConfig.Transforms) > 0 {
				t := existingConfig.Transforms[0]
				if t.Dest.Index == summaryIndex && strings.Contains(t.Description, traceSummaryVersion) {
					shouldCreate = false
					return nil
				}
				f.logger.Info("Transform version mismatch or config change. Recreating...", zap.String("id", transformID))
				f.deleteTransformJob(ctx, client, esURL, transformID)
				return nil
			}
			return fmt.Errorf("failed to parse transform check response: %s", string(bodyBytes))
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("unexpected status %d checking existing transform", resp.StatusCode)
	}()
	if err != nil {
		return err
	}

	if shouldCreate {
		// Since Jaeger doesn't store 'endTime', we calculate it dynamically (startTime + duration) to get accurate index boundaries.
		bodyStruct := map[string]any{
			"description": fmt.Sprintf("Jaeger Trace Summary - %s", traceSummaryVersion),
			"source": map[string]any{
				"index": fmt.Sprintf("%s-*", jaegerSpanIdx),
				"query": map[string]any{"match_all": map[string]any{}},
			},
			"dest": map[string]any{"index": summaryIndex},
			"sync": map[string]any{
				"time": map[string]any{"field": "startTimeMillis", "delay": "60s"},
			},
			"pivot": map[string]any{
				"group_by": map[string]any{
					"traceID": map[string]any{"terms": map[string]any{"field": "traceID"}},
				},
				"aggregations": map[string]any{
					"min_startTime": map[string]any{"min": map[string]any{"field": "startTime"}},
					"max_endTime": map[string]any{
						"scripted_metric": scriptedMetric{
							InitScript:    "state.timestamp = 0L",
							MapScript:     "long end = doc['startTime'].value + doc['duration'].value; if (end > state.timestamp) { state.timestamp = end }",
							CombineScript: "return state.timestamp",
							ReduceScript:  "long maxEnd = 0L; for (s in states) { if (s != null && s > maxEnd) { maxEnd = s } } return maxEnd",
						},
					},
				},
			},
		}
		bodyBytes, err := json.Marshal(bodyStruct)
		if err != nil {
			return fmt.Errorf("failed to marshal transform body: %w", err)
		}
		createURL := fmt.Sprintf("%s/_transform/%s", esURL, transformID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, createURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create transform request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to execute create request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("ES API error (status %d) and failed to read body: %w", resp.StatusCode, err)
			}
			return fmt.Errorf("ES API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		io.Copy(io.Discard, resp.Body)
	}

	return f.startTransformJob(ctx, client, esURL, transformID)
}

func (f *FactoryBase) startTransformJob(ctx context.Context, client *http.Client, esURL, transformID string) error {
	startURL := fmt.Sprintf("%s/_transform/%s/_start", esURL, transformID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build start request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// 409 means the transform is already running — this is expected and fine.
		f.logger.Debug("Transform already running, skipping start", zap.String("id", transformID))
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	if resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("failed to start transform %s (status %d): failed to read body: %w", transformID, resp.StatusCode, readErr)
		}
		return fmt.Errorf("failed to start transform %s (status %d): %s", transformID, resp.StatusCode, string(bodyBytes))
	}

	io.Copy(io.Discard, resp.Body)
	return nil
}

func (f *FactoryBase) deleteTransformJob(ctx context.Context, client *http.Client, esURL, transformID string) {
	runCommand := func(method, url string, ignore404 bool) {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			f.logger.Error("Failed to build cleanup request", zap.Error(err))
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			f.logger.Error("Network error during transform cleanup", zap.Error(err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			if ignore404 && resp.StatusCode == http.StatusNotFound {
				io.Copy(io.Discard, resp.Body)
				return
			}
			f.logger.Warn("Elasticsearch rejected cleanup command", zap.Int("status", resp.StatusCode), zap.String("id", transformID))
		}
		io.Copy(io.Discard, resp.Body)
	}

	stopURL := fmt.Sprintf("%s/_transform/%s/_stop?force=true&wait_for_completion=true", esURL, transformID)
	runCommand(http.MethodPost, stopURL, true)

	delURL := fmt.Sprintf("%s/_transform/%s", esURL, transformID)
	runCommand(http.MethodDelete, delURL, true)
}

func (f *FactoryBase) selectElasticsearchServer(ctx context.Context, servers []string, httpClient *http.Client) (string, error) {
	if len(servers) == 0 {
		return "", errors.New("no elasticsearch servers configured")
	}
	var lastErr error
	for _, s := range servers {
		found, err := func(serverURL string) (bool, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
			if err != nil {
				return false, err
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				return false, err
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				io.Copy(io.Discard, resp.Body)
				return true, nil
			}

			io.Copy(io.Discard, resp.Body)
			return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, serverURL)
		}(strings.TrimRight(s, "/"))

		if err == nil && found {
			return strings.TrimRight(s, "/"), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("no reachable elasticsearch servers: %w", lastErr)
}

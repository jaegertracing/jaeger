// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearchexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/collector/component/componenterror"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esmodeltranslator"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/storagemetrics"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esutil"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

const (
	spanIndexBaseName    = "jaeger-span"
	serviceIndexBaseName = "jaeger-service"
	spanTypeName         = "span"
	serviceTypeName      = "service"
)

// esSpanWriter holds components required for ES span writer
type esSpanWriter struct {
	logger           *zap.Logger
	nameTag          tag.Mutator
	client           esclient.ElasticsearchClient
	serviceCache     cache.Cache
	spanIndexName    esutil.IndexNameProvider
	serviceIndexName esutil.IndexNameProvider
	translator       *esmodeltranslator.Translator
	isArchive        bool
}

// newEsSpanWriter creates new instance of esSpanWriter
func newEsSpanWriter(params config.Configuration, logger *zap.Logger, archive bool, name string) (*esSpanWriter, error) {
	client, err := esclient.NewElasticsearchClient(params, logger)
	if err != nil {
		return nil, err
	}
	tagsKeysAsFields, err := params.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	alias := esutil.AliasNone
	if params.UseReadWriteAliases {
		alias = esutil.AliasWrite
	}
	return &esSpanWriter{
		logger:           logger,
		nameTag:          tag.Insert(storagemetrics.TagExporterName(), name),
		client:           client,
		spanIndexName:    esutil.NewIndexNameProvider(spanIndexBaseName, params.IndexPrefix, params.IndexDateLayout, alias, archive),
		serviceIndexName: esutil.NewIndexNameProvider(serviceIndexBaseName, params.IndexPrefix, params.IndexDateLayout, alias, archive),
		translator:       esmodeltranslator.NewTranslator(params.Tags.AllAsFields, tagsKeysAsFields, params.GetTagDotReplacement()),
		isArchive:        archive,
		serviceCache: cache.NewLRUWithOptions(
			// we do not expect more than 100k unique services
			100_000,
			&cache.Options{
				TTL: time.Hour * 12,
			},
		),
	}, nil
}

// CreateTemplates creates index templates.
func (w *esSpanWriter) CreateTemplates(ctx context.Context, spanTemplate, serviceTemplate string) error {
	err := w.client.PutTemplate(context.Background(), spanIndexBaseName, strings.NewReader(spanTemplate))
	if err != nil {
		return err
	}
	err = w.client.PutTemplate(ctx, serviceIndexBaseName, strings.NewReader(serviceTemplate))
	if err != nil {
		return err
	}
	return nil
}

// WriteTraces writes traces to the storage
func (w *esSpanWriter) WriteTraces(ctx context.Context, traces pdata.Traces) (int, error) {
	spans, err := w.translator.ConvertSpans(traces)
	if err != nil {
		return traces.SpanCount(), consumererror.Permanent(err)
	}
	return w.writeSpans(ctx, spans)
}

func (w *esSpanWriter) writeSpans(ctx context.Context, spansData []esmodeltranslator.ConvertedData) (int, error) {
	buffer := &bytes.Buffer{}
	// mapping for bulk operation to span
	var bulkItems []bulkItem
	var errs []error
	dropped := 0
	for _, spanData := range spansData {
		data, err := json.Marshal(spanData.DBSpan)
		if err != nil {
			errs = append(errs, err)
			dropped++
			continue
		}
		indexName := w.spanIndexName.IndexName(model.EpochMicrosecondsAsTime(spanData.DBSpan.StartTime))
		bulkItems = append(bulkItems, bulkItem{spanData: spanData, isService: false})
		w.client.AddDataToBulkBuffer(buffer, data, indexName, spanTypeName)
		if !w.isArchive {
			storeService, err := w.writeService(spanData.DBSpan, buffer)
			if err != nil {
				errs = append(errs, err)
				// dropped is not increased since this is only service name, the span could be written well
				continue
			} else if storeService {
				bulkItems = append(bulkItems, bulkItem{spanData: spanData, isService: true})
			}
		}
	}
	res, err := w.client.Bulk(ctx, buffer)
	if err != nil {
		errs = append(errs, err)
		return len(spansData), componenterror.CombineErrors(errs)
	}
	failedOperations, err := w.handleResponse(ctx, res, bulkItems)
	if err != nil {
		errs = append(errs, err)
	}
	dropped += len(failedOperations)
	if len(failedOperations) > 0 {
		return dropped, consumererror.PartialTracesError(componenterror.CombineErrors(errs), bulkItemsToTraces(failedOperations))
	}
	return dropped, componenterror.CombineErrors(errs)
}

// handleResponse processes blk response and returns spans that
func (w *esSpanWriter) handleResponse(ctx context.Context, blk *esclient.BulkResponse, bulkItems []bulkItem) ([]bulkItem, error) {
	storedSpans := map[string]int64{}
	notStoredSpans := map[string]int64{}
	var failed []bulkItem
	var errs []error
	for i, d := range blk.Items {
		bulkItem := bulkItems[i]
		if d.Index.Status > 201 {
			w.logger.Error("Part of the bulk request failed",
				zap.String("result", d.Index.Result),
				zap.String("error.reason", d.Index.Error.Reason),
				zap.String("error.type", d.Index.Error.Type),
				zap.String("error.cause.type", d.Index.Error.Cause.Type),
				zap.String("error.cause.reason", d.Index.Error.Cause.Reason))
			errs = append(errs, fmt.Errorf("bulk request failed, reason %v, result: %v", d.Index.Error.Reason, d.Index.Result))
			if !bulkItem.isService {
				failed = append(failed, bulkItem)
				notStoredSpans[bulkItem.spanData.DBSpan.Process.ServiceName] = notStoredSpans[bulkItem.spanData.DBSpan.Process.ServiceName] + 1
			}
		} else {
			// passed
			if !bulkItem.isService {
				storedSpans[bulkItem.spanData.DBSpan.Process.ServiceName] = storedSpans[bulkItem.spanData.DBSpan.Process.ServiceName] + 1
			} else {
				cacheKey := hashCode(bulkItem.spanData.DBSpan.Process.ServiceName, bulkItem.spanData.DBSpan.OperationName)
				w.serviceCache.Put(cacheKey, cacheKey)
			}
		}
	}
	for k, v := range notStoredSpans {
		ctx, _ := tag.New(ctx,
			tag.Insert(storagemetrics.TagServiceName(), k), w.nameTag)
		stats.Record(ctx, storagemetrics.StatSpansNotStoredCount().M(v))
	}
	for k, v := range storedSpans {
		ctx, _ := tag.New(ctx,
			tag.Insert(storagemetrics.TagServiceName(), k), w.nameTag)
		stats.Record(ctx, storagemetrics.StatSpansStoredCount().M(v))
	}
	return failed, multierror.Wrap(errs)
}

func (w *esSpanWriter) writeService(span *dbmodel.Span, buffer *bytes.Buffer) (bool, error) {
	cacheKey := hashCode(span.Process.ServiceName, span.OperationName)
	if w.serviceCache.Get(cacheKey) != nil {
		return false, nil
	}
	svc := dbmodel.Service{
		ServiceName:   span.Process.ServiceName,
		OperationName: span.OperationName,
	}
	data, err := json.Marshal(svc)
	if err != nil {
		return false, err
	}
	indexName := w.serviceIndexName.IndexName(model.EpochMicrosecondsAsTime(span.StartTime))
	w.client.AddDataToBulkBuffer(buffer, data, indexName, serviceTypeName)
	return true, nil
}

func hashCode(serviceName, operationName string) string {
	h := fnv.New64a()
	h.Write([]byte(serviceName))
	h.Write([]byte(operationName))
	return fmt.Sprintf("%x", h.Sum64())
}

type bulkItem struct {
	// span associated with the bulk operation
	spanData esmodeltranslator.ConvertedData
	// isService indicates that this bulk operation is for service index
	isService bool
}

func (w *esSpanWriter) esClientVersion() int {
	return w.client.MajorVersion()
}

func bulkItemsToTraces(bulkItems []bulkItem) pdata.Traces {
	traces := pdata.NewTraces()
	traces.ResourceSpans().Resize(len(bulkItems))
	for i, op := range bulkItems {
		spanData := op.spanData
		rss := traces.ResourceSpans().At(i)
		spanData.Resource.Attributes().CopyTo(rss.Resource().Attributes())
		rss.InstrumentationLibrarySpans().Resize(1)
		ispans := rss.InstrumentationLibrarySpans().At(0)
		spanData.InstrumentationLibrary.CopyTo(ispans.InstrumentationLibrary())
		ispans.Spans().Append(spanData.Span)
	}
	return traces
}

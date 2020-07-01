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

	"go.opentelemetry.io/collector/component/componenterror"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esmodeltranslator"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

const (
	spanIndexBaseName    = "jaeger-span"
	serviceIndexBaseName = "jaeger-service"
	spanTypeName         = "span"
	serviceTypeName      = "service"
	indexDateFormat      = "2006-01-02" // date format for index e.g. 2020-01-20
)

// esSpanWriter holds components required for ES span writer
type esSpanWriter struct {
	logger           *zap.Logger
	client           esclient.ElasticsearchClient
	serviceCache     cache.Cache
	spanIndexName    indexNameProvider
	serviceIndexName indexNameProvider
	translator       *esmodeltranslator.Translator
}

// newEsSpanWriter creates new instance of esSpanWriter
func newEsSpanWriter(params config.Configuration, logger *zap.Logger) (*esSpanWriter, error) {
	client, err := esclient.NewElasticsearchClient(params, logger)
	if err != nil {
		return nil, err
	}
	tagsKeysAsFields, err := config.LoadTagsFromFile(params.Tags.File)
	if err != nil {
		return nil, err
	}
	return &esSpanWriter{
		client:           client,
		spanIndexName:    newIndexNameProvider(spanIndexBaseName, params.IndexPrefix, params.UseReadWriteAliases),
		serviceIndexName: newIndexNameProvider(serviceIndexBaseName, params.IndexPrefix, params.UseReadWriteAliases),
		translator:       esmodeltranslator.NewTranslator(params.Tags.AllAsFields, tagsKeysAsFields, params.GetTagDotReplacement()),
		serviceCache: cache.NewLRUWithOptions(
			// we do not expect more than 100k unique services
			100_000,
			&cache.Options{
				TTL: time.Hour * 12,
			},
		),
	}, nil
}

func newIndexNameProvider(index, prefix string, useAliases bool) indexNameProvider {
	if prefix != "" {
		prefix = prefix + "-"
		index = prefix + index
	}
	index = index + "-"
	if useAliases {
		index = index + "write"
	}
	return indexNameProvider{
		index:    index,
		useAlias: useAliases,
	}
}

type indexNameProvider struct {
	index    string
	useAlias bool
}

func (n indexNameProvider) get(date time.Time) string {
	if n.useAlias {
		return n.index
	}
	spanDate := date.UTC().Format(indexDateFormat)
	return n.index + spanDate
}

// CreateTemplates creates index templates.
func (w *esSpanWriter) CreateTemplates(spanTemplate, serviceTemplate string) error {
	err := w.client.PutTemplate(spanIndexBaseName, strings.NewReader(spanTemplate))
	if err != nil {
		return err
	}
	err = w.client.PutTemplate(serviceIndexBaseName, strings.NewReader(serviceTemplate))
	if err != nil {
		return err
	}
	return nil
}

// WriteTraces writes traces to the storage
func (w *esSpanWriter) WriteTraces(_ context.Context, traces pdata.Traces) (int, error) {
	spans, err := w.translator.ConvertSpans(traces)
	if err != nil {
		return traces.SpanCount(), consumererror.Permanent(err)
	}
	return w.writeSpans(spans)
}

func (w *esSpanWriter) writeSpans(spans []*dbmodel.Span) (int, error) {
	buffer := &bytes.Buffer{}
	// mapping for bulk operation to span
	bulkOperations := make([]bulkItem, len(spans))
	var errs []error
	dropped := 0
	for _, span := range spans {
		data, err := json.Marshal(span)
		if err != nil {
			errs = append(errs, err)
			dropped++
			continue
		}
		indexName := w.spanIndexName.get(model.EpochMicrosecondsAsTime(span.StartTime))
		bulkOperations = append(bulkOperations, bulkItem{span: span, isService: false})
		w.client.AddDataToBulkBuffer(buffer, data, indexName, spanTypeName)
		write, err := w.writeService(span, buffer)
		if err != nil {
			errs = append(errs, err)
			// dropped is not increased since this is only service name, the span could be written well
			continue
		} else if write {
			bulkOperations = append(bulkOperations, bulkItem{span: span, isService: true})
		}
	}
	res, err := w.client.Bulk(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		errs = append(errs, err)
		return len(spans), componenterror.CombineErrors(errs)
	}
	droppedFromResponse := w.handleResponse(res, bulkOperations)
	dropped += droppedFromResponse
	return dropped, componenterror.CombineErrors(errs)
}

func (w *esSpanWriter) handleResponse(blk *esclient.BulkResponse, operationToSpan []bulkItem) int {
	numErrors := 0
	for i, d := range blk.Items {
		if d.Index.Status > 201 {
			numErrors++
			w.logger.Error("Part of the bulk request failed",
				zap.String("result", d.Index.Result),
				zap.String("error.reason", d.Index.Error.Reason),
				zap.String("error.type", d.Index.Error.Type),
				zap.String("error.cause.type", d.Index.Error.Cause.Type),
				zap.String("error.cause.reason", d.Index.Error.Cause.Reason))
			// TODO return an error or a struct that indicates which spans should be retried
			// https://github.com/open-telemetry/opentelemetry-collector/issues/990
		} else {
			// passed
			bulkOp := operationToSpan[i]
			if bulkOp.isService {
				cacheKey := hashCode(bulkOp.span.Process.ServiceName, bulkOp.span.OperationName)
				w.serviceCache.Put(cacheKey, cacheKey)
			}
		}
	}
	return numErrors
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
	indexName := w.serviceIndexName.get(model.EpochMicrosecondsAsTime(span.StartTime))
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
	span *dbmodel.Span
	// isService indicates that this bulk operation is for service index
	isService bool
}

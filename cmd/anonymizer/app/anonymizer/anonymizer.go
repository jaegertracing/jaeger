// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package anonymizer

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

var allowedTags = map[string]bool{
	"error":            true,
	"http.method":      true,
	"http.status_code": true,
	"span.kind":        true,
	"sampler.type":     true,
	"sampler.param":    true,
}

const PermUserRW = 0o600 // Read-write for owner only

// mapping stores the mapping of service/operation names to their one-way hashes,
// so that we can do a reverse lookup should the researchers have questions.
type mapping struct {
	Services   map[string]string
	Operations map[string]string // key=[service]:operation
}

// Anonymizer transforms a trace in the OTLP data model by obfuscating site-specific strings,
// like service and operation names, and by hashing or removing attributes.
//
// The mapping from original to obfuscated strings is stored in a file and can be reused between runs.
type Anonymizer struct {
	mappingFile string
	logger      *zap.Logger
	lock        sync.Mutex
	mapping     mapping
	options     Options
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// Options represents the various options with which the anonymizer can be configured.
type Options struct {
	HashStandardTags bool `yaml:"hash_standard_tags" name:"hash_standard_tags"`
	HashCustomTags   bool `yaml:"hash_custom_tags" name:"hash_custom_tags"`
	HashLogs         bool `yaml:"hash_logs" name:"hash_logs"`
	HashProcess      bool `yaml:"hash_process" name:"hash_process"`
}

// New creates new Anonymizer. The mappingFile stores the mapping from original to
// obfuscated strings, in case later investigations require looking at the original traces.
func New(mappingFile string, options Options, logger *zap.Logger) *Anonymizer {
	ctx, cancel := context.WithCancel(context.Background())
	a := &Anonymizer{
		mappingFile: mappingFile,
		logger:      logger,
		mapping: mapping{
			Services:   make(map[string]string),
			Operations: make(map[string]string),
		},
		options: options,
		cancel:  cancel,
	}
	if _, err := os.Stat(filepath.Clean(mappingFile)); err == nil {
		dat, err := os.ReadFile(filepath.Clean(mappingFile))
		if err != nil {
			logger.Fatal("Cannot load previous mapping", zap.Error(err))
		}
		if err := json.Unmarshal(dat, &a.mapping); err != nil {
			logger.Fatal("Cannot unmarshal previous mapping", zap.Error(err))
		}
	}
	a.wg.Go(func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.SaveMapping()
			case <-ctx.Done():
				return
			}
		}
	})
	return a
}

func (a *Anonymizer) Stop() {
	a.cancel()
	a.wg.Wait()
}

// SaveMapping writes the mapping from original to obfuscated strings to a file.
// It is called by the anonymizer itself periodically, and should be called at
// the end of the extraction run.
func (a *Anonymizer) SaveMapping() {
	a.lock.Lock()
	defer a.lock.Unlock()
	dat, err := json.Marshal(a.mapping)
	if err != nil {
		a.logger.Error("Failed to marshal mapping file", zap.Error(err))
		return
	}
	if err := os.WriteFile(filepath.Clean(a.mappingFile), dat, PermUserRW); err != nil {
		a.logger.Error("Failed to write mapping file", zap.Error(err))
		return
	}
	a.logger.Sugar().Infof("Saved mapping file %s: %s", a.mappingFile, string(dat))
}

func (a *Anonymizer) mapServiceName(service string) string {
	return a.mapString(service, a.mapping.Services)
}

func (a *Anonymizer) mapOperationName(service, operation string) string {
	v := fmt.Sprintf("[%s]:%s", service, operation)
	return a.mapString(v, a.mapping.Operations)
}

func (a *Anonymizer) mapString(v string, m map[string]string) string {
	a.lock.Lock()
	defer a.lock.Unlock()
	if s, ok := m[v]; ok {
		return s
	}
	s := hash(v)
	m[v] = s
	return s
}

func hash(value string) string {
	h := fnv.New64()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}

// AnonymizeTraces anonymizes the traces in place. What each option covers:
//   - resource attributes, the v1 process tags: hashed with HashProcess, otherwise dropped. The
//     service name is always kept, as its hash;
//   - span attributes: the standard ones (allowedTags) are kept, or hashed with HashStandardTags;
//     the rest are hashed with HashCustomTags, otherwise dropped;
//   - span events, the v1 logs: name and attributes hashed with HashLogs, otherwise dropped.
//
// OTLP carries some text v1 had no place for: schema URLs, the instrumentation scope, link
// attributes and the span status message. Each can hold site-specific strings, so each is treated as a custom
// attribute. The trace state, which v1 did not keep, is cleared. The span kind and status code are
// enumerations rather than text, so they are kept as they are.
func (a *Anonymizer) AnonymizeTraces(traces ptrace.Traces) {
	for _, rs := range traces.ResourceSpans().All() {
		rs.SetSchemaUrl(a.customText(rs.SchemaUrl()))
		service, hasService := rs.Resource().Attributes().Get(string(otelsemconv.ServiceNameKey))
		serviceName := ""
		if hasService {
			serviceName = service.AsString()
		}
		for _, ss := range rs.ScopeSpans().All() {
			ss.SetSchemaUrl(a.customText(ss.SchemaUrl()))
			a.anonymizeScope(ss.Scope())
			for _, span := range ss.Spans().All() {
				a.anonymizeSpan(serviceName, span)
			}
		}
		a.anonymizeResource(rs.Resource(), serviceName, hasService)
	}
}

func (a *Anonymizer) anonymizeResource(resource pcommon.Resource, service string, hasService bool) {
	attrs := resource.Attributes()
	if a.options.HashProcess {
		attrs.RemoveIf(func(key string, _ pcommon.Value) bool {
			return key == string(otelsemconv.ServiceNameKey)
		})
		hashAttributes(attrs)
	} else {
		attrs.Clear()
	}
	if hasService {
		attrs.PutStr(string(otelsemconv.ServiceNameKey), a.mapServiceName(service))
	}
}

func (a *Anonymizer) anonymizeScope(scope pcommon.InstrumentationScope) {
	scope.SetName(a.customText(scope.Name()))
	scope.SetVersion(a.customText(scope.Version()))
	a.anonymizeCustomAttributes(scope.Attributes())
}

func (a *Anonymizer) anonymizeSpan(service string, span ptrace.Span) {
	span.SetName(a.mapOperationName(service, span.Name()))
	a.anonymizeSpanAttributes(span.Attributes())
	span.TraceState().FromRaw("")
	span.Status().SetMessage(a.customText(span.Status().Message()))

	// when true, events are hashed, when false, they are dropped
	if a.options.HashLogs {
		for _, event := range span.Events().All() {
			event.SetName(hash(event.Name()))
			hashAttributes(event.Attributes())
		}
	} else {
		span.Events().RemoveIf(func(ptrace.SpanEvent) bool { return true })
	}

	for _, link := range span.Links().All() {
		link.TraceState().FromRaw("")
		a.anonymizeCustomAttributes(link.Attributes())
	}
}

// anonymizeSpanAttributes keeps the standard attributes ahead of the custom ones, which is the
// order the v1 anonymizer wrote tags in.
func (a *Anonymizer) anonymizeSpanAttributes(attrs pcommon.Map) {
	standard := pcommon.NewMap()
	custom := pcommon.NewMap()
	for key, value := range attrs.All() {
		if allowedTags[key] {
			value.CopyTo(standard.PutEmpty(key))
			if key == "error" {
				normalizeError(standard, value)
			}
		} else {
			value.CopyTo(custom.PutEmpty(key))
		}
	}
	// when true, the allowedTags are hashed and when false they are preserved as it is
	if a.options.HashStandardTags {
		hashAttributes(standard)
	}
	// when true, all tags other than allowedTags are hashed, when false they are dropped
	if a.options.HashCustomTags {
		hashAttributes(custom)
	} else {
		custom.Clear()
	}
	attrs.Clear()
	attrs.EnsureCapacity(standard.Len() + custom.Len())
	for key, value := range standard.All() {
		value.CopyTo(attrs.PutEmpty(key))
	}
	for key, value := range custom.All() {
		value.CopyTo(attrs.PutEmpty(key))
	}
}

// normalizeError keeps the error attribute a boolean, or a string that reads as one, and replaces
// anything else with true, so no free text survives under the one standard key that could hold it.
func normalizeError(attrs pcommon.Map, value pcommon.Value) {
	switch value.Type() {
	case pcommon.ValueTypeBool:
		return
	case pcommon.ValueTypeStr:
		if s := value.Str(); s == "true" || s == "false" {
			return
		}
	default:
	}
	attrs.PutBool("error", true)
}

// anonymizeCustomAttributes hashes attributes with HashCustomTags and drops them otherwise.
func (a *Anonymizer) anonymizeCustomAttributes(attrs pcommon.Map) {
	if a.options.HashCustomTags {
		hashAttributes(attrs)
	} else {
		attrs.Clear()
	}
}

// customText treats a piece of free text the way a custom attribute is treated: hashed with
// HashCustomTags and dropped otherwise. Empty text stays empty.
func (a *Anonymizer) customText(text string) string {
	if text == "" || !a.options.HashCustomTags {
		return ""
	}
	return hash(text)
}

// hashAttributes replaces every attribute with the hashes of its key and of its value as a string.
func hashAttributes(attrs pcommon.Map) {
	hashed := pcommon.NewMap()
	hashed.EnsureCapacity(attrs.Len())
	for key, value := range attrs.All() {
		hashed.PutStr(hash(key), hash(value.AsString()))
	}
	hashed.CopyTo(attrs)
}

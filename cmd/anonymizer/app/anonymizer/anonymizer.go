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

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"go.uber.org/zap"

	uiconv "github.com/jaegertracing/jaeger/model/converter/json"
	uimodel "github.com/jaegertracing/jaeger/model/json"
)

var allowedTags = map[string]bool{
	"error":               true,
	"http.method":         true,
	"http.status_code":    true,
	model.SpanKindKey:     true,
	model.SamplerTypeKey:  true,
	model.SamplerParamKey: true,
}

const PermUserRW = 0o600 // Read-write for owner only

// mapping stores the mapping of service/operation names to their one-way hashes,
// so that we can do a reverse lookup should the researchers have questions.
type mapping struct {
	Services   map[string]string
	Operations map[string]string // key=[service]:operation
}

// Anonymizer transforms Jaeger span in the domain model by obfuscating site-specific strings,
// like service and operation names, and removes custom tags. It returns obfuscated span in the
// Jaeger UI format, to make it easy to visualize traces.
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
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
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
	}()
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

// AnonymizeSpan obfuscates and converts the span.
func (a *Anonymizer) AnonymizeSpan(span *model.Span) *uimodel.Span {
	service := span.Process.ServiceName
	span.OperationName = a.mapOperationName(service, span.OperationName)

	outputTags := filterStandardTags(span.Tags)
	// when true, the allowedTags are hashed and when false they are preserved as it is
	if a.options.HashStandardTags {
		outputTags = hashTags(outputTags)
	}
	// when true, all tags other than allowedTags are hashed, when false they are dropped
	if a.options.HashCustomTags {
		customTags := hashTags(filterCustomTags(span.Tags))
		outputTags = append(outputTags, customTags...)
	}
	span.Tags = outputTags

	// when true, logs are hashed, when false, they are dropped
	if a.options.HashLogs {
		for _, log := range span.Logs {
			log.Fields = hashTags(log.Fields)
		}
	} else {
		span.Logs = nil
	}

	span.Process.ServiceName = a.mapServiceName(service)

	// when true, process tags are hashed, when false they are dropped
	if a.options.HashProcess {
		span.Process.Tags = hashTags(span.Process.Tags)
	} else {
		span.Process.Tags = nil
	}

	span.Warnings = nil
	return uiconv.FromDomainEmbedProcess(span)
}

// filterStandardTags returns only allowedTags
func filterStandardTags(tags []model.KeyValue) []model.KeyValue {
	out := make([]model.KeyValue, 0, len(tags))
	for _, tag := range tags {
		if !allowedTags[tag.Key] {
			continue
		}
		if tag.Key == "error" {
			switch tag.VType {
			case model.BoolType:
				// allowed
			case model.StringType:
				if tag.VStr != "true" && tag.VStr != "false" {
					tag = model.Bool("error", true)
				}
			default:
				tag = model.Bool("error", true)
			}
		}
		out = append(out, tag)
	}
	return out
}

// filterCustomTags returns all tags other than allowedTags
func filterCustomTags(tags []model.KeyValue) []model.KeyValue {
	out := make([]model.KeyValue, 0, len(tags))
	for _, tag := range tags {
		if !allowedTags[tag.Key] {
			out = append(out, tag)
		}
	}
	return out
}

// hashTags converts each tag into corresponding string values
// and then find its hash
func hashTags(tags []model.KeyValue) []model.KeyValue {
	out := make([]model.KeyValue, 0, len(tags))
	for _, tag := range tags {
		kv := model.String(hash(tag.Key), hash(tag.AsString()))
		out = append(out, kv)
	}
	return out
}

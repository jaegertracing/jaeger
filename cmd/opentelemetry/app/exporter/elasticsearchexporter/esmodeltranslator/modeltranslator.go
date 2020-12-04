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

package esmodeltranslator

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/translator/conventions"
	tracetranslator "go.opentelemetry.io/collector/translator/trace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

const (
	eventNameKey = "event"
)

var (
	errZeroTraceID     = errors.New("traceID is zero")
	errZeroSpanID      = errors.New("spanID is zero")
	emptyLogList       = []dbmodel.Log{}
	emptyReferenceList = []dbmodel.Reference{}
	emptyTagList       = []dbmodel.KeyValue{}
)

// Translator configures translator
type Translator struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
}

// NewTranslator returns new translator instance
func NewTranslator(allTagsAsFields bool, tagsKeysAsFields []string, tagDotReplacement string) *Translator {
	tagsKeysAsFieldsMap := map[string]bool{}
	for _, v := range tagsKeysAsFields {
		tagsKeysAsFieldsMap[v] = true
	}
	return &Translator{
		allTagsAsFields:   allTagsAsFields,
		tagKeysAsFields:   tagsKeysAsFieldsMap,
		tagDotReplacement: tagDotReplacement,
	}
}

// ConvertedData holds DB span and the original data used to construct it.
type ConvertedData struct {
	Span                   pdata.Span
	Resource               pdata.Resource
	InstrumentationLibrary pdata.InstrumentationLibrary
	DBSpan                 *dbmodel.Span
}

// ConvertSpans converts spans from OTEL model to Jaeger Elasticsearch model
func (c *Translator) ConvertSpans(traces pdata.Traces) ([]ConvertedData, error) {
	rss := traces.ResourceSpans()
	if rss.Len() == 0 {
		return nil, nil
	}
	spansData := make([]ConvertedData, 0, traces.SpanCount())
	for i := 0; i < rss.Len(); i++ {
		// this would correspond to a single batch
		err := c.resourceSpans(rss.At(i), &spansData)
		if err != nil {
			return nil, err
		}
	}
	return spansData, nil
}

func (c *Translator) resourceSpans(rspans pdata.ResourceSpans, spansData *[]ConvertedData) error {
	ils := rspans.InstrumentationLibrarySpans()
	process := c.process(rspans.Resource())
	for i := 0; i < ils.Len(); i++ {
		spans := ils.At(i).Spans()
		for j := 0; j < spans.Len(); j++ {
			dbSpan, err := c.spanWithoutProcess(spans.At(j))
			if err != nil {
				return err
			}
			c.addInstrumentationLibrary(dbSpan, ils.At(i).InstrumentationLibrary())
			dbSpan.Process = *process
			*spansData = append(*spansData, ConvertedData{
				Span:                   spans.At(j),
				Resource:               rspans.Resource(),
				InstrumentationLibrary: ils.At(i).InstrumentationLibrary(),
				DBSpan:                 dbSpan,
			})
		}
	}
	return nil
}

func (c *Translator) addInstrumentationLibrary(span *dbmodel.Span, instLib pdata.InstrumentationLibrary) {
	if instLib.Name() != "" {
		span.Tags = append(span.Tags, dbmodel.KeyValue{
			Key:   tracetranslator.TagInstrumentationName,
			Type:  dbmodel.StringType,
			Value: instLib.Name(),
		})
	}
	if instLib.Version() != "" {
		span.Tags = append(span.Tags, dbmodel.KeyValue{
			Key:   tracetranslator.TagInstrumentationVersion,
			Type:  dbmodel.StringType,
			Value: instLib.Version(),
		})
	}
}

func (c *Translator) spanWithoutProcess(span pdata.Span) (*dbmodel.Span, error) {
	traceID, err := convertTraceID(span.TraceID())
	if err != nil {
		return nil, err
	}
	spanID, err := convertSpanID(span.SpanID())
	if err != nil {
		return nil, err
	}
	references, err := references(span.Links(), span.ParentSpanID(), traceID)
	if err != nil {
		return nil, err
	}
	startTime := toTime(span.StartTime())
	startTimeMicros := model.TimeAsEpochMicroseconds(startTime)
	tags, tagMap := c.tags(span)
	return &dbmodel.Span{
		TraceID:         traceID,
		SpanID:          spanID,
		References:      references,
		OperationName:   span.Name(),
		StartTime:       startTimeMicros,
		StartTimeMillis: startTimeMicros / 1000,
		Duration:        model.DurationAsMicroseconds(toTime(span.EndTime()).Sub(startTime)),
		Tags:            tags,
		Tag:             tagMap,
		Logs:            logs(span.Events()),
	}, nil
}

func toTime(nano pdata.TimestampUnixNano) time.Time {
	return time.Unix(0, int64(nano)).UTC()
}

func references(links pdata.SpanLinkSlice, parentSpanID pdata.SpanID, traceID dbmodel.TraceID) ([]dbmodel.Reference, error) {
	parentSpanIDSet := parentSpanID.IsValid()
	if !parentSpanIDSet && links.Len() == 0 {
		return emptyReferenceList, nil
	}

	refsCount := links.Len()
	if parentSpanIDSet {
		refsCount++
	}

	refs := make([]dbmodel.Reference, 0, refsCount)

	// Put parent span ID at the first place because usually backends look for it
	// as the first CHILD_OF item in the model.SpanRef slice.
	if parentSpanIDSet {
		jParentSpanID, err := convertSpanID(parentSpanID)
		if err != nil {
			return nil, fmt.Errorf("OC incorrect parent span ID: %v", err)
		}
		refs = append(refs, dbmodel.Reference{
			TraceID: traceID,
			SpanID:  jParentSpanID,
			RefType: dbmodel.ChildOf,
		})
	}

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)

		traceID, err := convertTraceID(link.TraceID())
		if err != nil {
			continue // skip invalid link
		}

		spanID, err := convertSpanID(link.SpanID())
		if err != nil {
			continue // skip invalid link
		}

		refs = append(refs, dbmodel.Reference{
			TraceID: traceID,
			SpanID:  spanID,
			// Since Jaeger RefType is not captured in internal data,
			// use SpanRefType_FOLLOWS_FROM by default.
			// SpanRefType_CHILD_OF supposed to be set only from parentSpanID.
			RefType: dbmodel.FollowsFrom,
		})
	}
	return refs, nil
}

func convertSpanID(spanID pdata.SpanID) (dbmodel.SpanID, error) {
	if !spanID.IsValid() {
		return "", errZeroSpanID
	}
	src := spanID.Bytes()
	dst := make([]byte, hex.EncodedLen(len(src)))
	hex.Encode(dst, src[:])
	return dbmodel.SpanID(dst), nil
}

func convertTraceID(traceID pdata.TraceID) (dbmodel.TraceID, error) {
	if !traceID.IsValid() {
		return "", errZeroTraceID
	}
	high, low := tracetranslator.BytesToUInt64TraceID(traceID.Bytes())
	return dbmodel.TraceID(traceIDToString(high, low)), nil
}

func traceIDToString(high, low uint64) string {
	if high == 0 {
		return fmt.Sprintf("%016x", low)
	}
	return fmt.Sprintf("%016x%016x", high, low)
}

func (c *Translator) process(resource pdata.Resource) *dbmodel.Process {
	if resource.Attributes().Len() == 0 {
		return nil
	}
	p := &dbmodel.Process{}
	attrs := resource.Attributes()
	attrsCount := attrs.Len()
	if serviceName, ok := attrs.Get(conventions.AttributeServiceName); ok {
		p.ServiceName = serviceName.StringVal()
		attrsCount--
	}
	if attrsCount == 0 {
		return p
	}
	tags := make([]dbmodel.KeyValue, 0, attrsCount)
	var tagMap map[string]interface{}
	if c.allTagsAsFields || len(c.tagKeysAsFields) > 0 {
		tagMap = make(map[string]interface{}, attrsCount)
	}
	tags, tagMap = c.appendTagsFromAttributes(tags, tagMap, attrs, true)
	p.Tags = tags
	if len(tagMap) > 0 {
		p.Tag = tagMap
	}
	return p
}

func (c *Translator) tags(span pdata.Span) ([]dbmodel.KeyValue, map[string]interface{}) {
	var spanKindTag, statusCodeTag, errorTag, statusMsgTag dbmodel.KeyValue
	var spanKindTagFound, statusCodeTagFound, errorTagFound, statusMsgTagFound bool
	tagsCount := span.Attributes().Len()
	spanKindTag, spanKindTagFound = getTagFromSpanKind(span.Kind())
	if spanKindTagFound {
		tagsCount++
	}
	status := span.Status()
	if !status.IsNil() {
		statusCodeTag, statusCodeTagFound = getTagFromStatusCode(status.Code())
		tagsCount++

		errorTag, errorTagFound = getErrorTagFromStatusCode(status.Code())
		if errorTagFound {
			tagsCount++
		}

		statusMsgTag, statusMsgTagFound = getTagFromStatusMsg(status.Message())
		if statusMsgTagFound {
			tagsCount++
		}
	}
	if tagsCount == 0 {
		return emptyTagList, nil
	}
	tags := make([]dbmodel.KeyValue, 0, tagsCount)
	var tagMap map[string]interface{}
	if spanKindTagFound {
		if c.allTagsAsFields || c.tagKeysAsFields[spanKindTag.Key] {
			tagMap = c.addToTagMap(spanKindTag.Key, spanKindTag.Value, tagMap)
		} else {
			tags = append(tags, spanKindTag)
		}
	}
	if statusCodeTagFound {
		if c.allTagsAsFields || c.tagKeysAsFields[statusCodeTag.Key] {
			tagMap = c.addToTagMap(statusCodeTag.Key, statusCodeTag.Value, tagMap)
		} else {
			tags = append(tags, statusCodeTag)
		}
	}
	if errorTagFound {
		if c.allTagsAsFields || c.tagKeysAsFields[errorTag.Key] {
			tagMap = c.addToTagMap(errorTag.Key, errorTag.Value, tagMap)
		} else {
			tags = append(tags, errorTag)
		}
	}
	if statusMsgTagFound {
		if c.allTagsAsFields || c.tagKeysAsFields[statusMsgTag.Key] {
			tagMap = c.addToTagMap(statusMsgTag.Key, statusMsgTag.Value, tagMap)
		} else {
			tags = append(tags, statusMsgTag)
		}
	}
	return c.appendTagsFromAttributes(tags, tagMap, span.Attributes(), false)
}

func (c *Translator) addToTagMap(key string, val interface{}, tagMap map[string]interface{}) map[string]interface{} {
	if tagMap == nil {
		tagMap = map[string]interface{}{}
	}
	tagMap[strings.Replace(key, ".", c.tagDotReplacement, -1)] = val
	return tagMap
}

func getTagFromSpanKind(spanKind pdata.SpanKind) (dbmodel.KeyValue, bool) {
	var tagStr string
	switch spanKind {
	case pdata.SpanKindCLIENT:
		tagStr = string(tracetranslator.OpenTracingSpanKindClient)
	case pdata.SpanKindSERVER:
		tagStr = string(tracetranslator.OpenTracingSpanKindServer)
	case pdata.SpanKindPRODUCER:
		tagStr = string(tracetranslator.OpenTracingSpanKindProducer)
	case pdata.SpanKindCONSUMER:
		tagStr = string(tracetranslator.OpenTracingSpanKindConsumer)
	default:
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:   tracetranslator.TagSpanKind,
		Type:  dbmodel.StringType,
		Value: tagStr,
	}, true
}

func getTagFromStatusCode(statusCode pdata.StatusCode) (dbmodel.KeyValue, bool) {
	return dbmodel.KeyValue{
		Key:   tracetranslator.TagStatusCode,
		Value: statusCode.String(),
		Type:  dbmodel.StringType,
	}, true
}

func getErrorTagFromStatusCode(statusCode pdata.StatusCode) (dbmodel.KeyValue, bool) {
	if statusCode == pdata.StatusCode(0) {
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:   tracetranslator.TagError,
		Value: "true",
		Type:  dbmodel.BoolType,
	}, true
}

func getTagFromStatusMsg(statusMsg string) (dbmodel.KeyValue, bool) {
	if statusMsg == "" {
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:   tracetranslator.TagStatusMsg,
		Value: statusMsg,
		Type:  dbmodel.StringType,
	}, true
}

func logs(events pdata.SpanEventSlice) []dbmodel.Log {
	if events.Len() == 0 {
		return emptyLogList
	}
	logs := make([]dbmodel.Log, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		var fields []dbmodel.KeyValue
		if event.Attributes().Len() > 0 {
			fields = make([]dbmodel.KeyValue, 0, event.Attributes().Len()+1)
			if event.Name() != "" {
				fields = append(fields, dbmodel.KeyValue{Key: eventNameKey, Value: event.Name(), Type: dbmodel.StringType})
			}
			event.Attributes().ForEach(func(k string, v pdata.AttributeValue) {
				fields = append(fields, attributeToKeyValue(k, v))
			})
		}
		logs = append(logs, dbmodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(toTime(event.Timestamp())),
			Fields:    fields,
		})
	}
	return logs
}

func (c *Translator) appendTagsFromAttributes(tags []dbmodel.KeyValue, tagMap map[string]interface{}, attrs pdata.AttributeMap, skipService bool) ([]dbmodel.KeyValue, map[string]interface{}) {
	attrs.ForEach(func(key string, attr pdata.AttributeValue) {
		if skipService && key == conventions.AttributeServiceName {
			return
		}
		if c.allTagsAsFields || c.tagKeysAsFields[key] {
			tagMap = c.addToTagMap(key, attributeValueToInterface(attr), tagMap)
		} else {
			tags = append(tags, attributeToKeyValue(key, attr))
		}
	})
	return tags, tagMap
}

func attributeToKeyValue(key string, attr pdata.AttributeValue) dbmodel.KeyValue {
	tag := dbmodel.KeyValue{
		Key: key,
	}
	switch attr.Type() {
	case pdata.AttributeValueSTRING:
		tag.Type = dbmodel.StringType
		tag.Value = attr.StringVal()
	case pdata.AttributeValueBOOL:
		tag.Type = dbmodel.BoolType
		if attr.BoolVal() {
			tag.Value = "true"
		} else {
			tag.Value = "false"
		}
	case pdata.AttributeValueINT:
		tag.Type = dbmodel.Int64Type
		tag.Value = strconv.FormatInt(attr.IntVal(), 10)
	case pdata.AttributeValueDOUBLE:
		tag.Type = dbmodel.Float64Type
		tag.Value = strconv.FormatFloat(attr.DoubleVal(), 'g', 10, 64)
	}
	return tag
}

func attributeValueToInterface(attr pdata.AttributeValue) interface{} {
	switch attr.Type() {
	case pdata.AttributeValueSTRING:
		return attr.StringVal()
	case pdata.AttributeValueBOOL:
		return attr.BoolVal()
	case pdata.AttributeValueINT:
		return attr.IntVal()
	case pdata.AttributeValueDOUBLE:
		return attr.DoubleVal()
	}
	return nil
}

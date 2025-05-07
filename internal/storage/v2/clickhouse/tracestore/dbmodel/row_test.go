// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromEventsRow(t *testing.T) {
	trace := jsonToDBModel(t, "./fixtures/dbmodel.json")
	events := trace.Events
	eventsRow := jsonToEventsRow(t, "./fixtures/eventsRow.json")
	require.Len(t, events, len(eventsRow.Names))
	require.Len(t, events, len(eventsRow.Timestamps))
	require.Len(t, events, len(eventsRow.NestedAttrs))

	for i, event := range events {
		require.Equal(t, eventsRow.Names[i], event.Name)
		require.Equal(t, eventsRow.Timestamps[i], event.Timestamp)

		exceptedAttributes := FromNestedAttrRow(eventsRow.NestedAttrs[i])
		CompareAttributes(t, exceptedAttributes, event.Attributes)
	}
}

func TestFromLinksRow(t *testing.T) {
	trace := jsonToDBModel(t, "./fixtures/dbmodel.json")
	links := trace.Links
	linksRow := jsonToLinksRow(t, "./fixtures/linksRow.json")

	require.Len(t, links, len(linksRow.TraceIds))
	require.Len(t, links, len(linksRow.SpanIds))
	require.Len(t, links, len(linksRow.TraceStates))
	require.Len(t, links, len(linksRow.NestedAttrs))

	for i, link := range links {
		require.Equal(t, linksRow.TraceIds[i], link.TraceId)
		require.Equal(t, linksRow.SpanIds[i], link.SpanId)
		require.Equal(t, linksRow.TraceStates[i], link.TraceState)

		exceptedAttributes := FromNestedAttrRow(linksRow.NestedAttrs[i])
		CompareAttributes(t, exceptedAttributes, link.Attributes)
	}
}

func (row *NestedAttributesRow) UnmarshalJSON(data []byte) error {
	var rawAttrs map[string][][]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	err := decoder.Decode(&rawAttrs)
	if err != nil {
		return err
	}

	for key, pairs := range rawAttrs {
		switch key {
		case "BoolAttrs":
			row.BoolAttrs = make([][]any, len(pairs))
			for i, pair := range pairs {
				if len(pair) != 2 {
					return fmt.Errorf("BoolAttrs pair at index %d does not have 2 elements", i)
				}
				k, ok := pair[0].(string)
				if !ok {
					return fmt.Errorf("BoolAttrs pair at index %d is not a string", i)
				}
				v, ok := pair[1].(bool)
				if !ok {
					return fmt.Errorf("BoolAttrs pair at index %d is not a bool", i)
				}
				row.BoolAttrs[i] = []any{k, v}
			}
		case "DoubleAttrs":
			row.DoubleAttrs = make([][]any, len(pairs))
			for i, pair := range pairs {
				if len(pair) != 2 {
					return fmt.Errorf("DoubleAttrs pair at index %d does not have 2 elements", i)
				}
				k, ok := pair[0].(string)
				if !ok {
					return fmt.Errorf("DoubleAttrs pair at index %d is not a string", i)
				}
				v, err := pair[1].(json.Number).Float64()
				if err != nil {
					return fmt.Errorf("DoubleAttrs pair at index %d is not a number", i)
				}
				row.DoubleAttrs[i] = []any{k, v}
			}
		case "IntAttrs":
			row.IntAttrs = make([][]any, len(pairs))
			for i, pair := range pairs {
				if len(pair) != 2 {
					return fmt.Errorf("IntAttrs pair at index %d does not have 2 elements", i)
				}
				k, ok := pair[0].(string)
				if !ok {
					return fmt.Errorf("IntAttrs pair at index %d is not a string", i)
				}
				v, err := pair[1].(json.Number).Int64()
				if err != nil {
					return fmt.Errorf("IntAttrs pair at index %d is not a number", i)
				}
				row.IntAttrs[i] = []any{k, v}
			}
		case "StrAttrs":
			row.StrAttrs = make([][]any, len(pairs))
			for i, pair := range pairs {
				if len(pair) != 2 {
					return fmt.Errorf("StrAttrs pair at index %d does not have 2 elements", i)
				}
				k, ok := pair[0].(string)
				if !ok {
					return fmt.Errorf("StrAttrs pair at index %d is not a string", i)
				}
				v, ok := pair[1].(string)
				if !ok {
					return fmt.Errorf("StrAttrs pair at index %d is not a string", i)
				}
				row.StrAttrs[i] = []any{k, v}
			}
		case "BytesAttrs":
			row.BytesAttrs = make([][]any, len(pairs))
			for i, pair := range pairs {
				if len(pair) != 2 {
					return fmt.Errorf("BytesAttrs pair at index %d does not have 2 elements", i)
				}
				k, ok := pair[0].(string)
				if !ok {
					return fmt.Errorf("BytesAttrs pair at index %d is not a string", i)
				}
				v, ok := pair[1].(string)
				if !ok {
					return fmt.Errorf("BytesAttrs pair at index %d is not a base64 string", i)
				}
				bytesData, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					return fmt.Errorf("BytesAttrs pair at index %d is not a bytes", i)
				}
				row.BytesAttrs[i] = []any{k, bytesData}
			}
		}
	}
	return nil
}

func jsonToLinksRow(t *testing.T, filename string) *LinksRow {
	var lw *LinksRow
	data := readJSONBytes(t, filename)
	err := json.Unmarshal(data, &lw)
	require.NoError(t, err)
	return lw
}

func jsonToEventsRow(t *testing.T, filename string) *EventsRow {
	var ew *EventsRow
	data := readJSONBytes(t, filename)
	err := json.Unmarshal(data, &ew)
	require.NoError(t, err)
	return ew
}

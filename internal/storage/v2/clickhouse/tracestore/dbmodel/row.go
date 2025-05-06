// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"
)

// EventsRow is a collection of Trace.Events fields. It maps one-to-one with the fields in the table structure.
// When writing Trace.Events data, it needs to be first converted to the corresponding EventsRow. When reading, it's first mapped to EventsRow
// and then converted back to a collection of Trace.Events.
type EventsRow struct {
	Names         []string
	Timestamps    []time.Time
	NestedAttrRow []NestedAttributesRow
}

// LinksRow is a collection of Trace.Links fields. It maps one-to-one with the fields in the table structure.
// When writing Trace.Links data, it needs to be first converted to the corresponding LinksRow. When reading, it's first mapped to LinksRow
// and then converted back to Trace.Links.
type LinksRow struct {
	TraceIds      [][]byte
	SpanIds       [][]byte
	TraceStates   []string
	NestedAttrRow []NestedAttributesRow
}

// NestedAttributesRow is the actual storage form of AttributesGroup in Event and Link.
// AttributesGroup.IntKeys and AttributesGroup.IntValues are represented by their corresponding attribute slices here
// (e.g, IntAttrs holds IntKey/IntValue pairs). For example:
// AttributesGroup.StrKeys {"container.id", "container.name"},
// AttributesGroup.StrValues {"a3bf90e006b2", "jaeger"}
// will result in NestedAttributesRow.StrAttrs {{"container.id", "a3bf90e006b2"}, {"container.name", "jaeger"}}
type NestedAttributesRow struct {
	BoolAttrs   [][]any
	DoubleAttrs [][]any
	IntAttrs    [][]any
	StrAttrs    [][]any
	BytesAttrs  [][]any
}

// AllNestedAttrRow is a representation form for collections of NestedAttributesRow (like from EventsRow.NestedAttrRow or LinksRow.NestedAttrRow).
// Since we cannot access specific fields like NestedAttributesRow.StrAttrs directly when NestedAttributesRow is part of a slice (e.g []NestedAttributesRow),
// each attribute slice field within the collection of NestedAttributesRow is collected into independent arrays.
// For example, AllNestedAttrRow.StrAttrs will be an array of all StrAttrs slices from the input collection,
// which can then correspond to a field in the table structure (e.g., Array(Array(Nested(key, value)))) for writing/reading.
type AllNestedAttrRow struct {
	BoolAttrs   [][][]any
	DoubleAttrs [][][]any
	IntAttrs    [][][]any
	StrAttrs    [][][]any
	BytesAttrs  [][][]any
}

// ToEventsRow converts Trace.Events to EventsRow.
func ToEventsRow(events []Event) EventsRow {
	eventsRow := EventsRow{}

	for _, event := range events {
		eventsRow.Names = append(eventsRow.Names, event.Name)
		eventsRow.Timestamps = append(eventsRow.Timestamps, event.Timestamp)
		eventsRow.NestedAttrRow = append(eventsRow.NestedAttrRow, toNestedAttrRow(event.Attributes))
	}

	return eventsRow
}

// ToLinksRow converts Trace.Links to LinksRow
func ToLinksRow(links []Link) LinksRow {
	linksRow := LinksRow{}

	for _, link := range links {
		linksRow.TraceIds = append(linksRow.TraceIds, link.TraceId)
		linksRow.SpanIds = append(linksRow.SpanIds, link.SpanId)
		linksRow.TraceStates = append(linksRow.TraceStates, link.TraceState)
		linksRow.NestedAttrRow = append(linksRow.NestedAttrRow, toNestedAttrRow(link.Attributes))
	}

	return linksRow
}

// toNestedAttrRow converts Event.Attributes or Link.Attributes to NestedAttributesRow.
func toNestedAttrRow(attrs AttributesGroup) NestedAttributesRow {
	var nestedAttrRow NestedAttributesRow

	// For each attribute type, collect key-value pairs into slices of []any
	for i := range attrs.BoolKeys {
		boolAttrPair := []any{attrs.BoolKeys[i], attrs.BoolValues[i]}
		nestedAttrRow.BoolAttrs = append(nestedAttrRow.BoolAttrs, boolAttrPair)
	}

	for i := range attrs.DoubleKeys {
		doubleAttrPair := []any{attrs.DoubleKeys[i], attrs.DoubleValues[i]}
		nestedAttrRow.DoubleAttrs = append(nestedAttrRow.DoubleAttrs, doubleAttrPair)
	}

	for i := range attrs.IntKeys {
		intAttrPair := []any{attrs.IntKeys[i], attrs.IntValues[i]}
		nestedAttrRow.IntAttrs = append(nestedAttrRow.IntAttrs, intAttrPair)
	}

	for i := range attrs.StrKeys {
		stringAttrPair := []any{attrs.StrKeys[i], attrs.StrValues[i]}
		nestedAttrRow.StrAttrs = append(nestedAttrRow.StrAttrs, stringAttrPair)
	}

	for i := range attrs.BytesKeys {
		bytesAttrPair := []any{attrs.BytesKeys[i], attrs.BytesValues[i]}
		nestedAttrRow.BytesAttrs = append(nestedAttrRow.BytesAttrs, bytesAttrPair)
	}
	return nestedAttrRow
}

// FromNestedAttrRow converts NestedAttributesRow to Event.Attributes or Link.Attributes.
func FromNestedAttrRow(nestedAttr NestedAttributesRow) AttributesGroup {
	attributesGroup := AttributesGroup{}

	// Extract key-value pairs from the slices of []any
	for _, attrPair := range nestedAttr.BoolAttrs {
		boolKey := attrPair[0].(string)
		boolVal := attrPair[1].(bool)
		attributesGroup.BoolKeys = append(attributesGroup.BoolKeys, boolKey)
		attributesGroup.BoolValues = append(attributesGroup.BoolValues, boolVal)
	}
	for _, attrPair := range nestedAttr.DoubleAttrs {
		doubleKey := attrPair[0].(string)
		doubleVal := attrPair[1].(float64)
		attributesGroup.DoubleKeys = append(attributesGroup.DoubleKeys, doubleKey)
		attributesGroup.DoubleValues = append(attributesGroup.DoubleValues, doubleVal)
	}
	for _, attrPair := range nestedAttr.IntAttrs {
		intKey := attrPair[0].(string)
		intVal := attrPair[1].(int64)
		attributesGroup.IntKeys = append(attributesGroup.IntKeys, intKey)
		attributesGroup.IntValues = append(attributesGroup.IntValues, intVal)
	}
	for _, attrPair := range nestedAttr.StrAttrs {
		stringKey := attrPair[0].(string)
		stringVal := attrPair[1].(string)
		attributesGroup.StrKeys = append(attributesGroup.StrKeys, stringKey)
		attributesGroup.StrValues = append(attributesGroup.StrValues, stringVal)
	}
	for _, attrPair := range nestedAttr.BytesAttrs {
		bytesKey := attrPair[0].(string)
		bytesVal := attrPair[1].([]byte)
		attributesGroup.BytesKeys = append(attributesGroup.BytesKeys, bytesKey)
		attributesGroup.BytesValues = append(attributesGroup.BytesValues, bytesVal)
	}
	return attributesGroup
}

// ToAllNestedAttrRow converts a slice of NestedAttributesRow (like from EventsRow.NestedAttrRow or LinksRow.NestedAttrRow) to AllNestedAttrRow.
func ToAllNestedAttrRow(attrs []NestedAttributesRow) AllNestedAttrRow {
	var result AllNestedAttrRow
	// Collect each attribute slice from the input NestedAttributesRow instances into separate slices in AllNestedAttrRow
	for _, attr := range attrs {
		result.BoolAttrs = append(result.BoolAttrs, attr.BoolAttrs)
		result.DoubleAttrs = append(result.DoubleAttrs, attr.DoubleAttrs)
		result.IntAttrs = append(result.IntAttrs, attr.IntAttrs)
		result.StrAttrs = append(result.StrAttrs, attr.StrAttrs)
		result.BytesAttrs = append(result.BytesAttrs, attr.BytesAttrs)
	}
	return result
}

// FromAllNestedAttrRow converts AllNestedAttrRow back to a slice of NestedAttributesRow.
func FromAllNestedAttrRow(allNestedAttr AllNestedAttrRow) []NestedAttributesRow {
	// The length of the slices in AllNestedAttrRow indicates the number of original NestedAttributesRow instances
	result := make([]NestedAttributesRow, len(allNestedAttr.BoolAttrs))

	// Distribute the attribute slices back into NestedAttributesRow instances
	for i, attrSlice := range allNestedAttr.BoolAttrs {
		result[i].BoolAttrs = attrSlice
	}
	for i, attrSlice := range allNestedAttr.DoubleAttrs {
		result[i].DoubleAttrs = attrSlice
	}
	for i, attrSlice := range allNestedAttr.IntAttrs {
		result[i].IntAttrs = attrSlice
	}
	for i, attrSlice := range allNestedAttr.StrAttrs {
		result[i].StrAttrs = attrSlice
	}
	for i, attrSlice := range allNestedAttr.BytesAttrs {
		result[i].BytesAttrs = attrSlice
	}
	return result
}

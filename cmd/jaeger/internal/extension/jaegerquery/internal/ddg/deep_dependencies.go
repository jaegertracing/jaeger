// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package deepdependencies

type TDdgPayloadEntry struct {
	Service   string `json:"service"`
	Operation string `json:"operation"`
}

type Attribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TDdgPayloadPath struct {
	Path       []TDdgPayloadEntry `json:"path"`
	Attributes []Attribute        `json:"attributes"`
}

type TDdgPayload struct {
	Dependencies []TDdgPayloadPath `json:"dependencies"`
}

// GetData returns placeholder deep-dependencies data until a storage-backed
// implementation exists (#6606). Every identifier is deliberately synthetic —
// sample-service-*, sample-operation-* and sample-trace-id-* — so the graph
// cannot be mistaken for one computed from real traces, as #6606 requires. The
// queried focalService is kept as the graph's focal node so the UI still
// centers on it, but its neighbours and the exemplar IDs stay obviously fake.
func GetData(focalService string) TDdgPayload {
	if focalService == "" {
		focalService = "sample-focal-service"
	}

	return TDdgPayload{
		Dependencies: []TDdgPayloadPath{
			{
				Path: []TDdgPayloadEntry{
					{Service: focalService, Operation: "sample-operation-1"},
					{Service: "sample-service-a", Operation: "sample-operation-2"},
					{Service: "sample-service-b", Operation: "sample-operation-3"},
				},
				Attributes: []Attribute{{Key: "exemplar_trace_id", Value: "sample-trace-id-1"}},
			},
			{
				Path: []TDdgPayloadEntry{
					{Service: focalService, Operation: "sample-operation-1"},
					{Service: "sample-service-c", Operation: "sample-operation-4"},
					{Service: "sample-service-d", Operation: "sample-operation-5"},
				},
				Attributes: []Attribute{{Key: "exemplar_trace_id", Value: "sample-trace-id-2"}},
			},
			{
				Path: []TDdgPayloadEntry{
					{Service: "sample-service-e", Operation: "sample-operation-6"},
					{Service: focalService, Operation: "sample-operation-7"},
					{Service: "sample-service-f", Operation: "sample-operation-8"},
				},
				Attributes: []Attribute{{Key: "exemplar_trace_id", Value: "sample-trace-id-3"}},
			},
			{
				Path: []TDdgPayloadEntry{
					{Service: "sample-service-g", Operation: "sample-operation-9"},
					{Service: focalService, Operation: "sample-operation-10"},
					{Service: "sample-service-h", Operation: "sample-operation-11"},
					{Service: "sample-service-i", Operation: "sample-operation-12"},
				},
				Attributes: []Attribute{{Key: "exemplar_trace_id", Value: "sample-trace-id-4"}},
			},
		},
	}
}

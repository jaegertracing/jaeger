package integration

import (
	"testing"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"github.com/stretchr/testify/require"
	"encoding/json"
	"time"
)

func TestCheckTraceWithQuery_fixedTrace(t *testing.T) {
	inStr, err := ioutil.ReadFile("fixtures/trace.json")
	require.NoError(t, err)
	var trace model.Trace
	require.NoError(t, json.Unmarshal(inStr, &trace))

	testCases := []struct {
		caption string
		query *spanstore.TraceQueryParameters
		expected bool
	}{
		{
			caption: "service + startTime range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			caption: "service + operation + startTime range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				OperationName:  "test-general-conversion",
			},
			expected: true,
		},
		{
			caption: "service + startTime range + durationMax",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMax:    time.Duration(5000),
			},
			expected: true,
		},
		{
			caption: "service + startTime range + duration range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMin:    time.Duration(4900),
				DurationMax:    time.Duration(5100),
			},
			expected: true,
		},
		{
			caption: "service + operation + startTime range + durationMax",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMax:    time.Duration(5000),
				OperationName:  "test-general-conversion",
			},
			expected: true,
		},
		{
			caption: "service + operation + startTime range + duration range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMin:    time.Duration(4900),
				DurationMax:    time.Duration(5100),
				OperationName:  "test-general-conversion",
			},
			expected: true,
		},
		{
			caption: "service + startTime range + tags",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				Tags: map[string]string{"tag1":"value1", "tag2":"value2", "tag3":"value3"},
			},
			expected: true,
		},
		{
			caption: "service + operation + startTime range + tags",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				OperationName:  "some-operation",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				Tags: map[string]string{
					"peer.service":"service-y",
					"peer.ipv4":"23456",
					"temperature":"72.5",
					"error":"true",
				},
			},
			expected: true,
		},
		{
			caption: "tags in different spans in trace",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				OperationName:  "some-operation",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				Tags: map[string]string{
					"event":"some-event",
					"something":"blah",
					"temperature":"72.5",
					"tag1":"value1",
				},
			},
			expected: false, // TODO: we want this to be true.
		},
		{
			caption: "bad startTime range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 23, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 24, 0, 0, 0, time.UTC),
			},
			expected: false,
		},
		{
			caption: "bad service",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-unfound",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
			},
			expected: false,
		},
		{
			caption: "bad operation",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				OperationName:  "test-general-does-not-exist",
			},
			expected: false,
		},
		{
			caption: "bad durationMax",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMax:    time.Duration(900),
			},
			expected: false,
		},
		{
			caption: "bad duratio range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMin:    time.Duration(7500),
				DurationMax:    time.Duration(8000),
			},
			expected: false,
		},
		{
			caption: "service + operation match, but bad durationMax",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMax:    time.Duration(4900),
				OperationName:  "test-general-conversion",
			},
			expected: false,
		},
		{
			caption: "service + operation match, but bad duration range",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				DurationMin:    time.Duration(5100),
				DurationMax:    time.Duration(5500),
				OperationName:  "test-general-conversion",
			},
			expected: false,
		},
		{
			caption: "service and tags exist, but in different spans",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-y",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				Tags: map[string]string{"tag1":"value1", "tag2":"value2", "tag3":"value3"},
			},
			expected: false,
		},
		{
			caption: "bad tag value",
			query: &spanstore.TraceQueryParameters{
				ServiceName:	"service-x",
				OperationName:  "some-operation",
				StartTimeMin:   time.Date(2017, 1, 26, 21, 0, 0, 0, time.UTC),
				StartTimeMax:   time.Date(2017, 1, 26, 22, 0, 0, 0, time.UTC),
				Tags: map[string]string{
					"peer.service":"service-y",
					"peer.ipv4":"23456",
					"temperature":"72.7", // wrong value
					"error":"true",
				},
			},
			expected: false,
		},
	}

	for i, testCase := range testCases {
		actual := CheckTraceWithQuery(&trace, testCase.query)
		assert.Equal(t, testCase.expected, actual)
		if testCase.expected != actual {
			t.Logf("Failed case %d: %s", i, testCase.caption)
		}
	}
}
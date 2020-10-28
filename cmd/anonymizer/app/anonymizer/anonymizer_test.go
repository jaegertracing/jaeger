package anonymizer

import (
	"github.com/bmizerany/assert"
	"github.com/jaegertracing/jaeger/model"
	"testing"
	"time"

	uiconv "github.com/jaegertracing/jaeger/model/converter/json"
)

var tags = []model.KeyValue{
	model.KeyValue{
		Key:   "error",
		VType: model.BoolType,
		VBool: true,
	},
	model.KeyValue{
		Key:   "http.method",
		VType: model.StringType,
		VStr:  "POST",
	},
	model.KeyValue{
		Key:   "foobar",
		VType: model.BoolType,
		VBool: true,
	},
}

var traceID = model.NewTraceID(1, 2)

var span = &model.Span{
	TraceID: traceID,
	SpanID: model.NewSpanID(1),
	Process: &model.Process{
		ServiceName: "serviceName",
		Tags:        model.KeyValues{},
	},
	OperationName: "operationName",
	Tags: tags,
	Logs: []model.Log{
		{
			Timestamp: time.Now(),
			Fields: []model.KeyValue{
				model.String("logKey", "logValue"),
			},
		},
	},
	Duration:  time.Second * 5,
	StartTime: time.Unix(300, 0),
}

func TestAnonymizer_FilterStandardTags(t *testing.T) {
	expected := []model.KeyValue{
		model.KeyValue{
			Key: "error",
			VType: model.BoolType,
			VBool: true,
		},
		model.KeyValue{
			Key:   "http.method",
			VType: model.StringType,
			VStr:  "POST",
		},
	}

	actual := filterStandardTags(tags)
	assert.Equal(t, expected, actual)
}

func TestAnonymizer_FilterCustomTags(t *testing.T) {
	expected := []model.KeyValue{
		model.KeyValue{
			Key: "foobar",
			VType: model.BoolType,
			VBool: true,
		},
	}

	actual := filterCustomTags(tags)
	assert.Equal(t, expected, actual)
}

func TestAnonymizer_Hash(t *testing.T) {
	data := "foobar"
	expected := "340d8765a4dda9c2"
	actual := hash(data)
	assert.Equal(t, actual, expected)
}

func TestAnonymizer_AnonymizeSpan(t *testing.T) {
	anonymizer := &Anonymizer{
		mapping: mapping{
			Services:   make(map[string]string),
			Operations: make(map[string]string),
		},
		hashStandardTags: false,
		hashCustomTags: false,
		hashProcess: false,
		hashLogs: false,
	}

	actual := anonymizer.AnonymizeSpan(span)
	expected := uiconv.FromDomainEmbedProcess(span)
	assert.Equal(t, actual, expected)
}
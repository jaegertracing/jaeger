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

package anonymizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

var tags = []model.KeyValue{
	model.Bool("error", true),
	model.String("http.method", "POST"),
	model.Bool("foobar", true),
}

var traceID = model.NewTraceID(1, 2)

var span1 = &model.Span{
	TraceID: traceID,
	SpanID:  model.NewSpanID(1),
	Process: &model.Process{
		ServiceName: "serviceName",
		Tags:        tags,
	},
	OperationName: "operationName",
	Tags:          tags,
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

var span2 = &model.Span{
	TraceID: traceID,
	SpanID:  model.NewSpanID(1),
	Process: &model.Process{
		ServiceName: "serviceName",
		Tags:        tags,
	},
	OperationName: "operationName",
	Tags:          tags,
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

func TestNew(t *testing.T) {
	nopLogger := zap.NewNop()
	tempDir := t.TempDir()

	file, err := os.CreateTemp(tempDir, "mapping.json")
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(`
{
    "services": {
		"api": "hashed_api"
	},
	"operations": {
		"[api]:delete": "hashed_api_delete"
	}
}
`))
	require.NoError(t, err)

	anonymizer := New(file.Name(), Options{}, nopLogger)
	defer anonymizer.Stop()
	assert.NotNil(t, anonymizer)
}

func TestAnonymizer_SaveMapping(t *testing.T) {
	nopLogger := zap.NewNop()
	mapping := mapping{
		Services:   make(map[string]string),
		Operations: make(map[string]string),
	}

	tests := []struct {
		name        string
		mappingFile string
	}{
		{
			name:        "fail to write mapping file",
			mappingFile: "",
		},
		{
			name:        "save mapping file successfully",
			mappingFile: filepath.Join(t.TempDir(), "mapping.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			anonymizer := Anonymizer{
				logger:      nopLogger,
				mapping:     mapping,
				mappingFile: tt.mappingFile,
			}
			anonymizer.SaveMapping()
		})
	}
}

func TestAnonymizer_FilterStandardTags(t *testing.T) {
	expected := []model.KeyValue{
		model.Bool("error", true),
		model.String("http.method", "POST"),
	}
	actual := filterStandardTags(tags)
	assert.Equal(t, expected, actual)
}

func TestAnonymizer_FilterCustomTags(t *testing.T) {
	expected := []model.KeyValue{
		model.Bool("foobar", true),
	}
	actual := filterCustomTags(tags)
	assert.Equal(t, expected, actual)
}

func TestAnonymizer_Hash(t *testing.T) {
	data := "foobar"
	expected := "340d8765a4dda9c2"
	actual := hash(data)
	assert.Equal(t, expected, actual)
}

func TestAnonymizer_AnonymizeSpan_AllTrue(t *testing.T) {
	anonymizer := &Anonymizer{
		mapping: mapping{
			Services:   make(map[string]string),
			Operations: make(map[string]string),
		},
		options: Options{
			HashStandardTags: true,
			HashCustomTags:   true,
			HashProcess:      true,
			HashLogs:         true,
		},
	}
	_ = anonymizer.AnonymizeSpan(span1)
	assert.Len(t, span1.Tags, 3)
	assert.Len(t, span1.Logs, 1)
	assert.Len(t, span1.Process.Tags, 3)
}

func TestAnonymizer_AnonymizeSpan_AllFalse(t *testing.T) {
	anonymizer := &Anonymizer{
		mapping: mapping{
			Services:   make(map[string]string),
			Operations: make(map[string]string),
		},
		options: Options{
			HashStandardTags: false,
			HashCustomTags:   false,
			HashProcess:      false,
			HashLogs:         false,
		},
	}
	_ = anonymizer.AnonymizeSpan(span2)
	assert.Len(t, span2.Tags, 2)
	assert.Empty(t, span2.Logs)
	assert.Empty(t, span2.Process.Tags)
}

func TestAnonymizer_MapString_Present(t *testing.T) {
	v := "foobar"
	m := map[string]string{
		"foobar": "hashed_foobar",
	}
	anonymizer := &Anonymizer{}
	actual := anonymizer.mapString(v, m)
	assert.Equal(t, "hashed_foobar", actual)
}

func TestAnonymizer_MapString_Absent(t *testing.T) {
	v := "foobar"
	m := map[string]string{}
	anonymizer := &Anonymizer{}
	actual := anonymizer.mapString(v, m)
	assert.Equal(t, "340d8765a4dda9c2", actual)
}

func TestAnonymizer_MapServiceName(t *testing.T) {
	anonymizer := &Anonymizer{
		mapping: mapping{
			Services: map[string]string{
				"api": "hashed_api",
			},
		},
	}
	actual := anonymizer.mapServiceName("api")
	assert.Equal(t, "hashed_api", actual)
}

func TestAnonymizer_MapOperationName(t *testing.T) {
	anonymizer := &Anonymizer{
		mapping: mapping{
			Services: map[string]string{
				"api": "hashed_api",
			},
			Operations: map[string]string{
				"[api]:delete": "hashed_api_delete",
			},
		},
	}
	actual := anonymizer.mapOperationName("api", "delete")
	assert.Equal(t, "hashed_api_delete", actual)
}

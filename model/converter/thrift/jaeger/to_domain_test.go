// Copyright (c) 2017 Uber Technologies, Inc.
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

package jaeger

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

const NumberOfFixtures = 2

func TestToDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/thrift_batch_%02d.json", i)
		out := fmt.Sprintf("fixtures/domain_%02d.json", i)
		mSpans := loadSpans(t, out)
		for _, s := range mSpans {
			s.NormalizeTimestamps()
		}

		jBatch := loadBatch(t, in)
		name := in + " -> " + out + " : " + jBatch.Process.ServiceName
		t.Run(name, func(t *testing.T) {
			actualSpans := ToDomain(jBatch.Spans, jBatch.Process)
			for _, s := range actualSpans {
				s.NormalizeTimestamps()
			}
			if !assert.EqualValues(t, mSpans, actualSpans) {
				for _, err := range pretty.Diff(mSpans, actualSpans) {
					t.Log(err)
				}
				out, err := json.Marshal(actualSpans)
				assert.NoError(t, err)
				t.Logf("Actual trace %v: %s", i, string(out))
			}
		})
		if i == 1 {
			t.Run("ToDomainSpan", func(t *testing.T) {
				jSpan := jBatch.Spans[0]
				mSpan := ToDomainSpan(jSpan, jBatch.Process)
				mSpan.NormalizeTimestamps()
				assert.Equal(t, mSpans[0], mSpan)
			})
		}
	}
}

func loadSpans(t *testing.T, file string) []*model.Span {
	var trace model.Trace
	loadJSONPB(t, file, &trace)
	return trace.Spans
}

func loadJSONPB(t *testing.T, fileName string, obj proto.Message) {
	jsonFile, err := os.Open(fileName)
	require.NoError(t, err, "Failed to open json fixture file %s", fileName)
	require.NoError(t, jsonpb.Unmarshal(jsonFile, obj), fileName)
}

func loadBatch(t *testing.T, file string) *jaeger.Batch {
	var batch jaeger.Batch
	loadJSON(t, file, &batch)
	return &batch
}

func loadJSON(t *testing.T, fileName string, obj interface{}) {
	jsonFile, err := os.Open(fileName)
	require.NoError(t, err, "Failed to load json fixture file %s", fileName)
	jsonParser := json.NewDecoder(jsonFile)
	err = jsonParser.Decode(obj)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}

func TestUnknownJaegerType(t *testing.T) {
	mkv := toDomain{}.getTag(&jaeger.Tag{
		VType: 999,
		Key:   "sneh",
	})
	expected := model.String("sneh", "Unknown VType: Tag({Key:sneh VType:<UNSET> VStr:<nil> VDouble:<nil> VBool:<nil> VLong:<nil> VBinary:[]})")
	assert.Equal(t, mkv, expected)
}

func TestToDomain_ToDomainProcess(t *testing.T) {
	p := ToDomainProcess(&jaeger.Process{ServiceName: "foo", Tags: []*jaeger.Tag{{Key: "foo", VType: jaeger.TagType_BOOL}}})
	assert.Equal(t, &model.Process{ServiceName: "foo", Tags: []model.KeyValue{{Key: "foo", VType: model.BoolType}}}, p)
}

func TestToDomain_ToDomainSpanProcessNull(t *testing.T) {
	tm := time.Unix(158, 0)
	s := ToDomainSpan(&jaeger.Span{OperationName: "foo", StartTime: int64(model.TimeAsEpochMicroseconds(tm))}, nil)
	assert.Equal(t, &model.Span{OperationName: "foo", StartTime: tm.UTC()}, s)
}

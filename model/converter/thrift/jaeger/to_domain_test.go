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

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/thrift-gen/jaeger"
)

const NumberOfFixtures = 2

func TestToDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/thrift_batch_%02d.json", i)
		out := fmt.Sprintf("fixtures/model_%02d.json", i)
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
	var spans []*model.Span
	loadJSON(t, file, &spans)
	return spans
}

func loadBatch(t *testing.T, file string) *jaeger.Batch {
	var batch jaeger.Batch
	loadJSON(t, file, &batch)
	return &batch
}

func loadJSON(t *testing.T, fileName string, i interface{}) {
	jsonFile, err := os.Open(fileName)
	require.NoError(t, err, "Failed to load json fixture file %s", fileName)
	jsonParser := json.NewDecoder(jsonFile)
	err = jsonParser.Decode(i)
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

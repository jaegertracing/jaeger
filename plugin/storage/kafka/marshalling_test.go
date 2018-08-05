// Copyright (c) 2018 The Jaeger Authors.
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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtobufMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newProtobufMarshaller(), NewProtobufUnmarshaller())
}

func TestJSONMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newJSONMarshaller(), NewJSONUnmarshaller())
}

func testMarshallerAndUnmarshaller(t *testing.T, marshaller Marshaller, unmarshaller Unmarshaller) {
	bytes, err := marshaller.Marshal(sampleSpan)

	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	resultSpan, err := unmarshaller.Unmarshal(bytes)

	assert.NoError(t, err)
	assert.Equal(t, sampleSpan, resultSpan)
}

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package sanitizer

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/model"
	"go.uber.org/zap"
)

func invalidUTF8() string {
	s, _ := hex.DecodeString("fefeffff")
	return string(s)
}

var (
	invalidKV = model.KeyValues{
		model.KeyValue{
			Key:   invalidUTF8(),
			VType: model.StringType,
			VStr:  "value",
		},
	}
	undertest = NewUTF8Sanitizer(zap.NewNop())
)

func TestUTF8Sanitizer_SanitizeKV(t *testing.T) {

	tests := []struct {
		input    model.KeyValues
		expected model.KeyValues
	}{
		{
			input: model.KeyValues{
				model.String("valid key", "valid value"),
			},
			expected: model.KeyValues{
				model.String("valid key", "valid value"),
			},
		},
		{
			input: model.KeyValues{
				model.String(invalidUTF8(), "value"),
			},
			expected: model.KeyValues{
				model.Binary(invalidTagKey, []byte(invalidUTF8()+":value")),
			},
		},
		{
			input: model.KeyValues{
				model.String("valid key", invalidUTF8()),
			},
			expected: model.KeyValues{
				model.Binary("valid key", []byte(invalidUTF8())),
			},
		},
		{
			input: model.KeyValues{
				model.String(invalidUTF8(), invalidUTF8()),
			},
			expected: model.KeyValues{
				model.Binary(invalidTagKey, []byte(invalidUTF8()+":"+invalidUTF8())),
			},
		},
		{
			input: model.KeyValues{
				model.Binary(invalidService, []byte(invalidUTF8())),
			},
			expected: model.KeyValues{
				model.Binary(invalidService, []byte(invalidUTF8())),
			},
		},
	}
	for _, kv := range tests {
		sanitizeKV(kv.input)
		assert.Equal(t, kv.expected, kv.input)
	}
}

func TestUTF8Sanitizer_SanitizeServiceName(t *testing.T) {
	actual := undertest(
		&model.Span{
			Process: &model.Process{
				ServiceName: invalidUTF8(),
			},
		})
	assert.Equal(t, invalidService, actual.Process.ServiceName)
	assert.Equal(t, model.Binary(invalidService, []byte(invalidUTF8())), actual.Tags[0])
}

func TestUTF8Sanitizer_SanitizeOperationName(t *testing.T) {
	actual := undertest(&model.Span{
		OperationName: invalidUTF8(),
		Process:       &model.Process{},
	})
	assert.Equal(t, invalidOperation, actual.OperationName)
	assert.Equal(t, model.Binary(invalidOperation, []byte(invalidUTF8())), actual.Tags[0])
}

func TestUTF8Sanitizer_SanitizeProcessTags(t *testing.T) {
	actual := undertest(
		&model.Span{
			Process: &model.Process{
				Tags: invalidKV,
			},
		},
	)
	_, exists := actual.Process.Tags.FindByKey(invalidTagKey)
	assert.True(t, exists)

}

func TestUTF8Sanitizer_SanitizeTags(t *testing.T) {
	actual := undertest(
		&model.Span{
			Tags:    invalidKV,
			Process: &model.Process{},
		},
	)
	_, exists := actual.Tags.FindByKey(invalidTagKey)
	assert.True(t, exists)

}

func TestUTF8Sanitizer_SanitizeLogs(t *testing.T) {
	actual := undertest(
		&model.Span{
			Logs: []model.Log{
				{Fields: invalidKV},
			},
			Process: &model.Process{},
		},
	)
	assert.Equal(t, invalidTagKey, actual.Logs[0].Fields[0].Key)
}

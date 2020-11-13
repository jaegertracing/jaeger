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

package uiconv

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const spanData = `[{"traceId":"AAAAAAAAAABbgcpUtfnZBA==","spanId":"W4HKVLX52QQ=","operationName":"Meta::health","flags":1,"startTime":"2020-04-23T22:42:03.427158Z","duration":"0.000077s","process":{"serviceName":"acme"}},
{"traceId":"AAAAAAAAAABkQX7HRbUymw==","spanId":"E+85jzZSdoY=","operationName":"foobar","references":[{"traceId":"AAAAAAAAAABkQX7HRbUymw==","spanId":"Pgd+mTq/Zh4="}],"flags":1,"startTime":"2020-04-23T22:42:01.289306Z","duration":"0.024253s","process":{"serviceName":"xyz"}}
]`

func TestReader(t *testing.T) {
	f, err := ioutil.TempFile("", "captured-spans.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.Write([]byte(spanData))
	require.NoError(t, err)

	r, err := NewReader(
		f.Name(),
		zap.NewNop(),
	)
	require.NoError(t, err)

	s1, err := r.NextSpan()
	require.NoError(t, err)
	assert.Equal(t, "Meta::health", s1.OperationName)

	s2, err := r.NextSpan()
	require.NoError(t, err)
	assert.Equal(t, "foobar", s2.OperationName)

	_, err = r.NextSpan()
	require.Equal(t, io.EOF, err)
}

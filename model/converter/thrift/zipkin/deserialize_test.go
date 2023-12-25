// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package zipkin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestDeserializeWithBadListStart(t *testing.T) {
	ctx := context.Background()
	spanBytes := SerializeThrift(ctx, []*zipkincore.Span{{}})
	_, err := DeserializeThrift(ctx, append([]byte{0, 255, 255}, spanBytes...))
	require.Error(t, err)
}

func TestDeserializeWithCorruptedList(t *testing.T) {
	ctx := context.Background()
	spanBytes := SerializeThrift(ctx, []*zipkincore.Span{{}})
	spanBytes[2] = 255
	_, err := DeserializeThrift(ctx, spanBytes)
	require.Error(t, err)
}

func TestDeserialize(t *testing.T) {
	ctx := context.Background()
	spanBytes := SerializeThrift(ctx, []*zipkincore.Span{{}})
	_, err := DeserializeThrift(ctx, spanBytes)
	require.NoError(t, err)
}

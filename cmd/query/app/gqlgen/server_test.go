package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/jaegertracing/jaeger/pkg/otel/trace/v1"
)

func TestNewJSON(t *testing.T) {
	s := &v1.Span{
		Name: "hello",
		TraceId: []byte{0, 1, 2, 3, 4},
	}
	jsonpb := v1.JSONPb{}
	json, err := jsonpb.Marshal(s)
	require.NoError(t, err)
	fmt.Println(string(json))
}

package jaeger

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/thrift-gen/jaeger"
)

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

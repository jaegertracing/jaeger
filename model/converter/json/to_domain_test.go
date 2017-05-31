package json_test

import (
	"testing"
	"fmt"
	"io/ioutil"
	"encoding/json"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
	jModel "github.com/uber/jaeger/model/json"
	. "github.com/uber/jaeger/model/converter/json"

	"bytes"
)


func TestToDomainES(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/es_%02d.json", i)
		inStr, err := ioutil.ReadFile(in)
		require.NoError(t, err)
		var span jModel.ESSpan
		require.NoError(t, json.Unmarshal(inStr, &span))

		actualSpan, err := ToDomainES(&span)
		require.NoError(t, err)

		out := fmt.Sprintf("fixtures/domain_es_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, json.Unmarshal(outStr, &expectedSpan))

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		require.NoError(t, enc.Encode(actualSpan))
		actual := string(buf.Bytes())

		if !assert.Equal(t, string(outStr), actual) {
			err := ioutil.WriteFile(out+"-actual", buf.Bytes(), 0644)
			assert.NoError(t, err)
		}

		//if !assert.ObjectsAreEqualValues(actualSpan, expectedSpan) {
		//	t.Logf("blah%s", actualSpan.StartTime)
		//	buf := &bytes.Buffer{}
		//	enc := json.NewEncoder(buf)
		//	enc.SetIndent("", "  ")
		//	require.NoError(t, enc.Encode(actualSpan))
		//
		//	err := ioutil.WriteFile(out+"-actual", buf.Bytes(), 0644)
		//	assert.NoError(t, err)
		//}
	}
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), model.Float64("x", 72.5).VNum)
}
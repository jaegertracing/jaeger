package metrics

import (
	"net/http"
	"testing"

	"flag"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBind(t *testing.T) {
	b := &Builder{}
	flags := flag.NewFlagSet("foo", flag.PanicOnError)
	b.Bind(flags)
}

func TestBuilder(t *testing.T) {
	testCases := []struct {
		backend string
		route   string
		err     error
		handler bool
	}{
		{
			backend: "expvar",
			route:   "/",
			handler: true,
		},
		{
			backend: "prometheus",
			route:   "/",
			handler: true,
		},
		{
			backend: "none",
			handler: false,
		},
		{
			backend: "",
			handler: false,
		},
		{
			backend: "invalid",
			err:     errUnknownBackend,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		b := &Builder{
			Backend:   testCase.backend,
			HTTPRoute: testCase.route,
		}
		mf, err := b.CreateMetricsFactory("foo")
		if testCase.err != nil {
			assert.Equal(t, err, testCase.err)
			continue
		}
		require.NotNil(t, mf)
		if testCase.handler {
			require.NotNil(t, b.handler)
			mux := http.NewServeMux()
			b.RegisterHandler(mux)
		}
	}
}

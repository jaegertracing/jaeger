package client

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, r.Body)
	}))
	defer server.Close()

	client := &SimpleClient{
		Endpoint: server.URL,
		Cli:      http.DefaultClient,
	}

	var buf bytes.Buffer

	payload := "hello world"

	buf.Write([]byte(payload))

	err := client.Post(&buf)
	assert.NoError(t, err)
}

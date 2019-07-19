package integration

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func Test_bearTokenPropagationHandler(t *testing.T)  {
	hostName := "query.127.0.0.1.nip.io"
	authClient, err := newOauthAuthenticatedClient(hostName, "developer")
	if err != nil {
		print(err.Error())
	}
	assert.Nil(t, err)
	// Do a request for search service, should not return forbidden.
	resp, err := getResponse("https://"+ hostName + "/api/services", authClient)
	if err != nil {
		print(err.Error())
	}
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK,resp.StatusCode)
}


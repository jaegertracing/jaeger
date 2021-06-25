package hostname

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsIdentifier(t *testing.T) {
	hostname1, err := AsIdentifier()
	require.NoError(t, err)
	hostname2, err := AsIdentifier()
	require.NoError(t, err)

	actualHostname, _ := os.Hostname()

	assert.NotEqual(t, hostname1, hostname2)
	assert.True(t, strings.HasPrefix(hostname1, actualHostname))
	assert.True(t, strings.HasPrefix(hostname2, actualHostname))
}

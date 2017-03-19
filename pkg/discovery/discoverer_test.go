package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixedDiscoverer(t *testing.T) {
	d := FixedDiscoverer([]string{"a", "b"})
	instances, err := d.Instances()
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, instances)
}

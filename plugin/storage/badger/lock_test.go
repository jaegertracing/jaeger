package badger

import (
	"testing"
	"time"

	"github.com/crossdock/crossdock-go/assert"
)

func TestAcquire(t *testing.T) {
	l := &lock{}
	ok, err := l.Acquire("resource", time.Duration(1))
	assert.True(t, ok)
	assert.NoError(t, err)
}

func TestForfeit(t *testing.T) {
	l := &lock{}
	ok, err := l.Forfeit("resource")
	assert.True(t, ok)
	assert.NoError(t, err)
}

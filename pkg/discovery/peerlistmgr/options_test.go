package peerlistmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/zap"
)

func TestOptions(t *testing.T) {
	logger := zap.New(zap.NullEncoder())
	minPeers := 42
	connCheck := 42 * time.Millisecond
	connTimeout := 42 * time.Millisecond
	opts := Options.apply(
		Options.Logger(logger),
		Options.MinPeers(minPeers),
		Options.ConnCheckFrequency(connCheck),
		Options.ConnCheckTimeout(connTimeout),
	)
	assert.Equal(t, logger, opts.logger)
	assert.Equal(t, minPeers, opts.minPeers)
	assert.Equal(t, connCheck, opts.connCheckFrequency)
	assert.Equal(t, connTimeout, opts.connCheckTimeout)
}

func TestOptionsDefaults(t *testing.T) {
	opts := Options.apply()
	assert.NotNil(t, opts.logger)
	assert.Equal(t, defaultMinPeers, opts.minPeers)
	assert.Equal(t, defaultConnCheckFrequency, opts.connCheckFrequency)
	assert.Equal(t, defaultConnCheckTimeout, opts.connCheckTimeout)
}

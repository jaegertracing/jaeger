package testutils

import (
	"bytes"

	"github.com/uber-go/zap"
)

// NewLogger creates a new zap.Logger backed by a bytes.Buffer, which is also returned.
func NewLogger(json bool) (zap.Logger, *bytes.Buffer) {
	var encoder zap.Encoder
	if json {
		encoder = zap.NewJSONEncoder(zap.NoTime())
	} else {
		encoder = zap.NewTextEncoder(zap.TextNoTime())
	}
	buf := &bytes.Buffer{}
	return zap.New(
		encoder,
		zap.Output(zap.AddSync(buf)),
		zap.DebugLevel,
	), buf
}

package testutils

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/zap"
)

func TestNewTextLogger(t *testing.T) {
	logger, log := NewLogger(false)
	logger.Warn("hello", zap.String("x", "y"))
	v := string(log.Bytes())
	assert.Equal(t, "[W] hello x=y\n", v)
}

func TestNewJSONLogger(t *testing.T) {
	logger, log := NewLogger(true)
	logger.Warn("hello", zap.String("x", "y"))

	data := make(map[string]string)
	require.NoError(t, json.Unmarshal(log.Bytes(), &data))
	delete(data, "time")
	assert.Equal(t, map[string]string{
		"level": "warn",
		"msg":   "hello",
		"x":     "y",
	}, data)
}

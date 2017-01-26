package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestTimeConversion(t *testing.T) {
	s := model.TimeAsEpochMicroseconds(time.Unix(100, 2000))
	assert.Equal(t, uint64(100000002), s)
	assert.Equal(t, time.Unix(100, 2000), model.EpochMicrosecondsAsTime(s))
}

func TestDurationConversion(t *testing.T) {
	d := model.DurationAsMicroseconds(12345 * time.Microsecond)
	assert.Equal(t, 12345*time.Microsecond, model.MicrosecondsAsDuration(d))
	assert.Equal(t, uint64(12345), d)
}

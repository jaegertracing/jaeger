package setupcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupContext(t *testing.T) {
	// purely for code coverage
	SetAllInOne()
	defer UnsetAllInOne()
	assert.True(t, IsAllInOne())
}

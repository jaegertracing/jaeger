package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommand(t *testing.T) {
	cmd := Command()

	assert.NotNil(t, cmd, "Command() should return a non-nil *cobra.Command instance")

	expectedShortDescription := "Jaeger backend v2"
	assert.Equal(t, expectedShortDescription, cmd.Short, "Command short description should be '%s'", expectedShortDescription)

	expectedLongDescription := "Jaeger backend v2"
	assert.Equal(t, expectedLongDescription, cmd.Long, "Command long description should be '%s'", expectedLongDescription)

	assert.NotNil(t, cmd.RunE, "Command should have RunE function set")

}

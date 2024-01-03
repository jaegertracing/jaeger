package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponents(t *testing.T) {
	factories, err := components()

	assert.NoError(t, err)

	assert.NotNil(t, factories.Extensions)
	assert.NotNil(t, factories.Receivers)
	assert.NotNil(t, factories.Exporters)
	assert.NotNil(t, factories.Processors)
	assert.NotNil(t, factories.Connectors)

	_, jaegerReceiverFactoryExists := factories.Receivers["jaeger"]
	assert.True(t, jaegerReceiverFactoryExists)
}

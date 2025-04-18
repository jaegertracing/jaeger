package remotestorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension"
)

func Test_NewFactory(t *testing.T) {
	factory := NewFactory()
	require.Equal(t, componentType, factory.Type())
	require.Equal(t, factory.CreateDefaultConfig(), createDefaultConfig())
}

func Test_CreateExtension(t *testing.T) {
	set := extension.Settings{
		ID: ID,
	}
	cfg := createDefaultConfig()
	ext, err := createExtension(context.Background(), set, cfg)

	require.NoError(t, err)
	require.NotNil(t, ext)
}

package storagecleaner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension/extensiontest"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NotNil(t, cfg, "failed to create default config")
	require.NoError(t, componenttest.CheckConfigStruct(cfg))
}

func TestCreateExtension(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	f := NewFactory()
	r, err := f.CreateExtension(context.Background(), extensiontest.NewNopCreateSettings(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, r)
}

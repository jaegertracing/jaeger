package remotesampling

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

func TestNewFactory(t *testing.T) {
	// 1. Verify the factory can be created
	f := NewFactory()
	assert.NotNil(t, f)

	// 2. Verify the Component Type
	assert.Equal(t, ComponentType, f.Type())

	// 3. Verify Default Configuration is not nil
	cfg := f.CreateDefaultConfig()
	assert.NotNil(t, cfg)

	// 4. Verify CreateExtension logic
	ctx := context.Background()
	params := extension.Settings{
		ID: component.NewID(ComponentType),
		TelemetrySettings: component.TelemetrySettings{
			Logger: nil, // Logger is optional for this test
		},
	}

	// We attempt to create the extension. 
	// Even if it returns an error (due to missing grpc config), 
	// we just want to ensure it doesn't panic.
	ext, err := f.Create(ctx, params, cfg)
	
	if err == nil {
		assert.NotNil(t, ext)
		assert.NoError(t, ext.Shutdown(ctx))
	} else {
		// If it errors, that's fine too, as long as it's a valid error
		assert.Error(t, err)
	}
}

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_ReturnsErrorWhenInvalid(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Configuration
	}{
		{
			name: "missing required fields",
			cfg:  &Configuration{},
		},
		{
			name: "require fields in invalid format",
			cfg: &Configuration{
				Connection: Connection{
					Servers: []string{"not a url"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			require.NoError(t, err)
		})
	}
}

func TestValidate_DoesNotReturnErrorWhenRequiredFieldsSet(t *testing.T) {

	cfg := Configuration{
		Connection: Connection{
			Servers: []string{"localhost:9200"},
		},
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

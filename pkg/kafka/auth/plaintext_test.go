// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPlainTextConfiguration(t *testing.T) {
	tests := []struct {
		name              string
		config            PlainTextConfig
		expectedError     error
		expectedMechanism sarama.SASLMechanism
	}{
		{
			name: "SCRAM-SHA-256",
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "SCRAM-SHA-256",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypeSCRAMSHA256,
		},
		{
			name: "SCRAM-SHA-512",
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "SCRAM-SHA-512",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypeSCRAMSHA512,
		},
		{
			name: "PLAIN",
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "PLAIN",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypePlaintext,
		},
		{
			name: "Invalid Mechanism",
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "INVALID",
			},
			expectedError: fmt.Errorf("config plaintext.mechanism error: %s, only support 'SCRAM-SHA-256' or 'SCRAM-SHA-512' or 'PLAIN'", "INVALID"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saramaConfig := sarama.NewConfig()

			err := setPlainTextConfiguration(&tt.config, saramaConfig)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.True(t, saramaConfig.Net.SASL.Enable)
				assert.Equal(t, tt.config.Username, saramaConfig.Net.SASL.User)
				assert.Equal(t, tt.config.Password, saramaConfig.Net.SASL.Password)
				assert.Equal(t, tt.expectedMechanism, saramaConfig.Net.SASL.Mechanism)

				if strings.HasPrefix(tt.config.Mechanism, "SCRAM-SHA-") {
					assert.NotNil(t, saramaConfig.Net.SASL.SCRAMClientGeneratorFunc)
				} else {
					assert.Nil(t, saramaConfig.Net.SASL.SCRAMClientGeneratorFunc)
				}
			}
		})
	}
}

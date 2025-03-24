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
	"github.com/xdg-go/scram"
)

func TestScramClient(t *testing.T) {
	scramClientFunc := clientGenFunc(scram.SHA256)
	client := scramClientFunc().(*scramClient)

	err := client.Begin("testUser", "testPassword", "testAuthzID")
	require.NoError(t, err, "Begin should not return an error")
	assert.NotNil(t, client.Client, "Client should be initialized")
	assert.NotNil(t, client.ClientConversation, "ClientConversation should be initialized")

	step, err := client.Step("testChallenge")
	require.NoError(t, err, "Step should not return an error")
	require.NotEmpty(t, step, "Step should return a non-empty response")

	done := client.Done()
	assert.False(t, done, "Done should return false initially")
}

func TestSetPlainTextConfiguration(t *testing.T) {
	tests := []struct {
		config            PlainTextConfig
		expectedError     error
		expectedMechanism sarama.SASLMechanism
	}{
		{
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "SCRAM-SHA-256",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypeSCRAMSHA256,
		},
		{
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "SCRAM-SHA-512",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypeSCRAMSHA512,
		},
		{
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "PLAIN",
			},
			expectedError:     nil,
			expectedMechanism: sarama.SASLTypePlaintext,
		},
		{
			config: PlainTextConfig{
				Username:  "username",
				Password:  "password",
				Mechanism: "INVALID_MECHANISM",
			},
			expectedError: fmt.Errorf("config plaintext.mechanism error: %s, only support 'SCRAM-SHA-256' or 'SCRAM-SHA-512' or 'PLAIN'", "INVALID_MECHANISM"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.config.Mechanism, func(t *testing.T) {
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

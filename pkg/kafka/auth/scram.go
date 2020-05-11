// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"crypto/sha512"
	"fmt"
	"hash"

	"github.com/Shopify/sarama"
	scrampkg "github.com/xdg/scram"
)

// ScramConfig describes the configuration properties required for the SCRAM handshake
// between the collector and the pipeline
type ScramConfig struct {
	UserName  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	Algorithm string `mapstructure:"algorithm"`
}

// SetSCRAMConfiguration ...
func setSCRAMConfiguration(config *ScramConfig, saramaConfig *sarama.Config) error {
	var fn func() sarama.SCRAMClient

	var mechanism sarama.SASLMechanism

	switch config.Algorithm {
	case "sha512":
		fn = func() sarama.SCRAMClient {
			return &scramClient{
				HashGeneratorFcn: func() hash.Hash { return sha512.New() },
			}
		}
		mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA512)
	case "sha256":
		fn = func() sarama.SCRAMClient {
			return &scramClient{
				HashGeneratorFcn: scrampkg.SHA256,
			}
		}
		mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA256)
	default:
		return fmt.Errorf("invalid SHA algorithm '%s': can be either 'sha256' or 'sha512'", config.Algorithm)
	}
	saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = fn
	saramaConfig.Net.SASL.Mechanism = mechanism
	saramaConfig.Net.SASL.Enable = true
	saramaConfig.Net.SASL.User = config.UserName
	saramaConfig.Net.SASL.Password = config.Password

	return nil
}

type scramClient struct {
	*scrampkg.Client
	*scrampkg.ClientConversation
	scrampkg.HashGeneratorFcn
}

// Begin uses a username password and authid to generate a new client and instantiate a new conversation
func (client *scramClient) Begin(userName, password, authzID string) (err error) {
	client.Client, err = client.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return fmt.Errorf("Begin method failed on: %s", err)
	}
	client.ClientConversation = client.Client.NewConversation()
	return nil
}

// Step takes a challenge string
func (client *scramClient) Step(challenge string) (response string, err error) {
	response, err = client.ClientConversation.Step(challenge)
	if err != nil {
		return "", fmt.Errorf("Step method failed on: %s", err)

	}
	return response, nil
}

// Done returns a bool based on a completed conversation
func (client *scramClient) Done() bool {
	return client.ClientConversation.Done()
}

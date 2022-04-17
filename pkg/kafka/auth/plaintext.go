// Copyright (c) 2019 The Jaeger Authors.
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
	"fmt"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/xdg-go/scram"
)

// scramClient is the client to use when the auth mechanism is SCRAM
type scramClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

// Begin prepares the client for the SCRAM exchange
// with the server with a user name and a password
func (x *scramClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

// Step steps client through the SCRAM exchange. It is
// called repeatedly until it errors or `Done` returns true.
func (x *scramClient) Step(challenge string) (response string, err error) {
	response, err = x.ClientConversation.Step(challenge)
	return
}

// Done should return true when the SCRAM conversation
// is over.
func (x *scramClient) Done() bool {
	return x.ClientConversation.Done()
}

// PlainTextConfig describes the configuration properties needed for SASL/PLAIN with kafka
type PlainTextConfig struct {
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password" json:"-"`
	Mechanism string `mapstructure:"mechanism"`
}

var _ sarama.SCRAMClient = (*scramClient)(nil)

func clientGenFunc(hashFn scram.HashGeneratorFcn) func() sarama.SCRAMClient {
	return func() sarama.SCRAMClient {
		return &scramClient{HashGeneratorFcn: hashFn}
	}
}

func setPlainTextConfiguration(config *PlainTextConfig, saramaConfig *sarama.Config) error {
	saramaConfig.Net.SASL.Enable = true
	saramaConfig.Net.SASL.User = config.Username
	saramaConfig.Net.SASL.Password = config.Password
	switch strings.ToUpper(config.Mechanism) {
	case "SCRAM-SHA-256":
		saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = clientGenFunc(scram.SHA256)
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = clientGenFunc(scram.SHA512)
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
	case "PLAIN":
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext

	default:
		return fmt.Errorf("config plaintext.mechanism error: %s, only support 'SCRAM-SHA-256' or 'SCRAM-SHA-512' or 'PLAIN'", config.Mechanism)

	}
	return nil
}

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
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/xdg/scram"
)

// used for return a *sarama.SCRAMClient on create SCRAMClientGeneratorFunc when the mechanism is SCRAM-SHA-256 or SCRAM-SHA-512
type XDGSCRAMClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

func (x *XDGSCRAMClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

func (x *XDGSCRAMClient) Step(challenge string) (response string, err error) {
	response, err = x.ClientConversation.Step(challenge)
	return
}

func (x *XDGSCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}

// PlainTextConfig describes the configuration properties needed for SASL/PLAIN with kafka
type PlainTextConfig struct {
	UserName  string `mapstructure:"username"`
	Password  string `mapstructure:"password" json:"-"`
	Mechanism string `mapstructure:"mechanism"`
}

func setPlainTextConfiguration(config *PlainTextConfig, saramaConfig *sarama.Config) error {
	saramaConfig.Net.SASL.Enable = true
	saramaConfig.Net.SASL.User = config.UserName
	saramaConfig.Net.SASL.Password = config.Password
	switch strings.ToUpper(config.Mechanism) {
	case "SCRAM-SHA-256":
		saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &XDGSCRAMClient{HashGeneratorFcn: func() hash.Hash { return sha256.New() }}
		}
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &XDGSCRAMClient{HashGeneratorFcn: func() hash.Hash { return sha512.New() }}
		}
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
	case "PLAIN":
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext

	default:
		return fmt.Errorf("config plaintext.mechanism error: %s, only support 'SCRAM-SHA-256' or 'SCRAM-SHA-512' or 'PLAIN'", config.Mechanism)

	}
	return nil
}

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

import (
	"github.com/olivere/elastic"
	"github.com/pkg/errors"

	"github.com/uber/jaeger/pkg/es"
)

// Configuration describes the configuration properties needed to connect to a ElasticSearch cluster
type Configuration struct {
	Servers  []string
	username string
	password string
	sniffer  bool
}

// NewClient creates a new ElasticSearch client
func (c *Configuration) NewClient() (es.Client, error) {
	if len(c.Servers) < 1 {
		return nil, errors.New("No servers specified")
	}
	rawClient, err := elastic.NewClient(c.GetConfigs()...)
	if err != nil {
		return nil, err
	}
	return es.WrapESClient(rawClient), nil
}

// GetConfigs wraps the configs to feed to the ElasticSearch client init
func (c *Configuration) GetConfigs() []elastic.ClientOptionFunc {
	options := make([]elastic.ClientOptionFunc, 3)
	options = append(options, elastic.SetURL(c.Servers...))
	options = append(options, elastic.SetBasicAuth(c.username, c.password))
	options = append(options, elastic.SetSniff(c.sniffer))
	return options
}

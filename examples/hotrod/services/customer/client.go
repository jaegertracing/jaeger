// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package customer

import (
	"context"
	"fmt"
	"net/http"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
)

// Client is a remote client that implements customer.Interface
type Client struct {
	tracer   opentracing.Tracer
	logger   log.Factory
	client   *tracing.HTTPClient
	hostPort string
}

// NewClient creates a new customer.Client
func NewClient(tracer opentracing.Tracer, logger log.Factory, hostPort string) *Client {
	return &Client{
		tracer: tracer,
		logger: logger,
		client: &tracing.HTTPClient{
			Client: &http.Client{Transport: &nethttp.Transport{}},
			Tracer: tracer,
		},
		hostPort: hostPort,
	}
}

// Get implements customer.Interface#Get as an RPC
func (c *Client) Get(ctx context.Context, customerID string) (*Customer, error) {
	c.logger.For(ctx).Info("Getting customer", zap.String("customer_id", customerID))

	url := fmt.Sprintf("http://"+c.hostPort+"/customer?customer=%s", customerID)
	fmt.Println(url)
	var customer Customer
	if err := c.client.GetJSON(ctx, "/customer", url, &customer); err != nil {
		return nil, err
	}
	return &customer, nil
}

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

package driver

import (
	"context"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/services/driver/thrift-gen/driver"
)

// Client is a remote client that implements driver.Interface
type Client struct {
	tracer opentracing.Tracer
	logger log.Factory
	ch     *tchannel.Channel
	client driver.TChanDriver
}

// NewClient creates a new driver.Client
func NewClient(tracer opentracing.Tracer, logger log.Factory) *Client {
	channelOpts := &tchannel.ChannelOptions{
		//Logger:        logger,
		//StatsReporter: statsReporter,
		Tracer: tracer,
	}
	ch, err := tchannel.NewChannel("driver-client", channelOpts)
	if err != nil {
		logger.Bg().Fatal("Cannot create TChannel", zap.Error(err))
	}
	clientOpts := &thrift.ClientOptions{
		HostPort: "127.0.0.1:8082",
	}
	thriftClient := thrift.NewClient(ch, "driver", clientOpts)
	client := driver.NewTChanDriverClient(thriftClient)

	return &Client{
		tracer: tracer,
		logger: logger,
		ch:     ch,
		client: client,
	}
}

// FindNearest implements driver.Interface#FindNearest as an RPC
func (c *Client) FindNearest(ctx context.Context, location string) ([]Driver, error) {
	c.logger.For(ctx).Info("Finding nearest drivers", zap.String("location", location))
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	results, err := c.client.FindNearest(thrift.Wrap(ctx), location)
	if err != nil {
		return nil, err
	}
	return fromThrift(results), nil
}

func fromThrift(results []*driver.DriverLocation) []Driver {
	retMe := make([]Driver, len(results))
	for i, result := range results {
		retMe[i] = Driver{
			DriverID: result.DriverID,
			Location: result.Location,
		}
	}
	return retMe
}

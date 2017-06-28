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

package app

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

func TestAgentStartError(t *testing.T) {
	cfg := &Builder{}
	agent, err := cfg.CreateAgent(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	agent.httpServer.Addr = "bad-address"
	assert.Error(t, agent.Run())
}

func TestAgentStartStop(t *testing.T) {
	cfg := Builder{
		Processors: []ProcessorConfiguration{
			{
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					HostPort: "127.0.0.1:0",
				},
			},
		},
	}
	agent, err := cfg.CreateAgent(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	ch := make(chan error, 2)
	go func() {
		if err := agent.Run(); err != nil {
			t.Errorf("error from agent.Run(): %s", err)
			ch <- err
		}
		close(ch)
	}()

	url := fmt.Sprintf("http://%s/sampling?service=abc", agent.httpServer.Addr)
	httpClient := &http.Client{
		Timeout: 100 * time.Millisecond,
	}
	for i := 0; i < 1000; i++ {
		_, err := httpClient.Get(url)
		if err == nil {
			break
		}
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("error from agent: %s", err)
			}
			break
		default:
			time.Sleep(time.Millisecond)
		}
	}
	resp, err := http.Get(url)
	require.NoError(t, err)
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "tcollector error: no peers available\n", string(body))

	// TODO (wjang) We sleep because the processors in the agent might not have had time to
	// start up yet. If we were to call Stop() before the processors have time to startup,
	// it'll panic because of a DATA RACE between wg.Add() and wg.Wait() in thrift_processor.
	// A real fix for this issue would be to add semaphores to tbuffered_server, thrift_processor,
	// and agent itself. Given all this extra overhead and testing required to get this to work
	// "elegantly", I opted to just sleep here given how unlikely this situation will occur in
	// production.
	time.Sleep(2 * time.Second)

	agent.Stop()
	assert.NoError(t, <-ch)
}

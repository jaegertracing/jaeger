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
	"net"
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
	agent.samplingServer.Addr = "bad-address"
	assert.Error(t, agent.Run())
}

func TestAgentStartStop(t *testing.T) {
	cfg := Builder{
		Processors: []ProcessorConfiguration{
			{
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					HostPort: "127.0.0.1:5776",
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

	url := fmt.Sprintf("http://%s/sampling?service=abc", agent.samplingServer.Addr)
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

	for i := 0; i < 1000; i++ {
		destAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:5776")
		t.Log(destAddr.String())
		if err == nil {
			_, err = net.DialUDP(destAddr.Network(), nil, destAddr)
			if err == nil {
				break
			} else {
				t.Log(err.Error())
			}
		} else {
			t.Log(err.Error())
		}
		time.Sleep(time.Millisecond)
	}

	agent.Stop()
	assert.NoError(t, <-ch)
}

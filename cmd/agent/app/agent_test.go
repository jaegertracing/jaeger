package app

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"
)

func TestAgentStartError(t *testing.T) {
	cfg := &Builder{}
	agent, err := cfg.CreateAgent(metrics.NullFactory, zap.New(zap.NullEncoder()))
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
					HostPort: "127.0.0.1:0",
				},
			},
		},
	}
	agent, err := cfg.CreateAgent(metrics.NullFactory, zap.New(zap.NullEncoder()))
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
	agent.Stop()
	assert.NoError(t, <-ch)
}

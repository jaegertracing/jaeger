// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"flag"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

var errActionTest = errors.New("action error")

type dummyAction struct {
	TestFn func() error
}

func (a *dummyAction) Do() error {
	return a.TestFn()
}

func TestExecuteAction(t *testing.T) {
	tests := []struct {
		name                  string
		flags                 []string
		expectedExecuteAction bool
		expectedSkip          bool
		expectedError         error
		actionFunction        func() error
		configError           bool
	}{
		{
			name: "execute errored action",
			flags: []string{
				"--es.tls.skip-host-verify=true",
			},
			expectedExecuteAction: true,
			expectedSkip:          true,
			expectedError:         errActionTest,
		},
		{
			name: "execute success action",
			flags: []string{
				"--es.tls.skip-host-verify=true",
			},
			expectedExecuteAction: true,
			expectedSkip:          true,
			expectedError:         nil,
		},
		{
			name: "don't action because error in tls options",
			flags: []string{
				"--es.tls.cert=/invalid/path/for/cert",
			},
			expectedExecuteAction: false,
			configError:           true,
		},
	}
	logger := zap.NewNop()
	args := []string{
		"https://localhost:9300",
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			tlsFlags := tlscfg.ClientFlagsConfig{Prefix: "es"}
			command := cobra.Command{}
			flags := &flag.FlagSet{}
			tlsFlags.AddFlags(flags)
			command.PersistentFlags().AddGoFlagSet(flags)
			v.BindPFlags(command.PersistentFlags())
			cmdLine := append([]string{"--es.tls.enabled=true"}, test.flags...)
			err := command.ParseFlags(cmdLine)
			require.NoError(t, err)
			executedAction := false
			err = ExecuteAction(ActionExecuteOptions{
				Args:     args,
				Viper:    v,
				Logger:   logger,
				TLSFlags: tlsFlags,
			}, func(c client.Client, _ Config) Action {
				assert.Equal(t, "https://localhost:9300", c.Endpoint)
				transport, ok := c.Client.Transport.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
				return &dummyAction{
					TestFn: func() error {
						executedAction = true
						return test.expectedError
					},
				}
			})

			assert.Equal(t, test.expectedExecuteAction, executedAction)
			if test.configError {
				require.Error(t, err)
			} else {
				assert.Equal(t, test.expectedError, err)
			}
		})
	}
}

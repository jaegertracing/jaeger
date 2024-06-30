// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	//"flag"
	//"strings"
	"testing"

	//"github.com/Shopify/sarama"
	//"github.com/spf13/cobra"
	//"github.com/spf13/viper"

	//"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	//"fmt"
	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSetConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	// saramaConfig := sarama.NewConfig()
	// configPrefix := "kafka.auth"
	// flagSet := flag.NewFlagSet("flags", flag.ContinueOnError)
	// auth.AddFlags(configPrefix, flagSet)
	// command := &cobra.Command{}
	// command.Flags().AddGoFlagSet(flagSet)
	// v := viper.New()
	// v.AutomaticEnv()
	// v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	// v.BindPFlags(command.Flags())

	// authConfig := &auth.AuthenticationConfig{}
	// command.ParseFlags([]string{
	// 	"--kafka.auth.authentication=fail",
	// })

	// err := authConfig.InitFromViper(configPrefix, v)
	// require.NoError(t, err)
	// require.Error(t, authConfig.SetConfiguration(saramaConfig, logger), "Unknown/Unsupported authentication method fail to kafka cluster")
	test := &Configuration{AuthenticationConfig: auth.AuthenticationConfig{Authentication: "fail"}}
	_, err := test.NewConsumer(logger)
	require.EqualError(t, err,"Unknown/Unsupported authentication method fail to kafka cluster")
}

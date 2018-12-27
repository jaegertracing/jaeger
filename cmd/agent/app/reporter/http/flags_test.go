package http

import (
    "time"
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestBingFlags(t *testing.T) {
	tests := []struct {
		flags   []string
		builder Builder
	}{
		{
			flags: []string{
			"--reporter.http.collector-endpoint=1.2.3.4:555",
			"--reporter.http.request-timeout=10s",
		}, builder: Builder{CollectorEndpoint: "1.2.3.4:555", RequestTimeout: time.Second * 10},
		},
		{
			flags: []string{
			"--reporter.http.collector-endpoint=1.2.3.4:555",
			},
            builder: Builder{CollectorEndpoint: "1.2.3.4:555", RequestTimeout: defaultRequestTimeout},
		},
		{
			flags: []string{
			"--reporter.http.request-timeout=10s",
			},
            builder: Builder{RequestTimeout: time.Second * 10},
		},
	}
	for _, test := range tests {
		// Reset flags every iteration.
		v := viper.New()
		command := cobra.Command{}

		flags := &flag.FlagSet{}
		AddFlags(flags)
		command.ResetFlags()
		command.PersistentFlags().AddGoFlagSet(flags)
		v.BindPFlags(command.PersistentFlags())

		err := command.ParseFlags(test.flags)
		require.NoError(t, err)
		b := Builder{}
		b.InitFromViper(v)
		assert.Equal(t, test.builder, b)
	}
}

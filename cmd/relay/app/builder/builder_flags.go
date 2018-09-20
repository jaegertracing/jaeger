package builder

import (
	"flag"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	receiverJaegerTChannelPort = "receiver.jaeger.tchannel-port"
	receiverJaegerHTTPPort     = "receiver.jaeger.http-port"
	receiverZipkinHTTPort      = "receiver.zipkin.http-port"

	reporterTChannelHostPort                  = "reporter.tchannel.host-port"
	reporterTChannelDiscoveryMinPeers         = "reporter.tchannel.discovery.min-peers"
	reporterTChannelDiscoveryConnCheckTimeout = "reporter.tchannel.discovery.conn-check-timeout"

	// RelayDefaultHealthCheckHTTPPort is the default HTTP Port for health check
	RelayDefaultHealthCheckHTTPPort = 14269

	defaultMinPeers         = 3
	defaultConnCheckTimeout = 250 * time.Millisecond
)

// ReceiverOptions holds configuration for receivers
type ReceiverOptions struct {
	// ReceiverJaegerTchannelPort is the port that the relay receives on for tchannel requests
	ReceiverJaegerTChannelPort int
	// ReceiverJaegerHTTPPort is the port that the relay receives on for http requests
	ReceiverJaegerHTTPPort int
	// ReceiverZipkinHTTPPort is the port that the relay receives on for zipkin http requests
	ReceiverZipkinHTTPPort int
}

// ReporterOptions holds configuration for reporters
type ReporterOptions struct {
	ReporterTChannelHostPorts                 []string
	ReporterTChannelDiscoveryMinPeers         int
	ReporterTChannelDiscoveryConnCheckTimeout time.Duration
}

// AddFlags adds flags for ReceiverOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(receiverJaegerTChannelPort, 14267, "The tchannel port for the Jaeger receiver service")
	flags.Int(receiverJaegerHTTPPort, 14268, "The http port for the Jaeger receiver service")
	flags.Int(receiverZipkinHTTPort, 9411, "The http port for the Zipkin reciver service e.g. 9411")

	flags.String(
		reporterTChannelHostPort,
		"",
		"comma-separated string representing host:ports of a static list of collectors to connect to directly (e.g. when not using service discovery)")
	flags.Int(
		reporterTChannelDiscoveryMinPeers,
		defaultMinPeers,
		"if using service discovery, the min number of connections to maintain to the backend")
	flags.Duration(
		reporterTChannelDiscoveryConnCheckTimeout,
		defaultConnCheckTimeout,
		"sets the timeout used when establishing new connections")
}

// InitFromViper initializes CollectorOptions with properties from viper
func (rOpts *ReceiverOptions) InitFromViper(v *viper.Viper) *ReceiverOptions {
	rOpts.ReceiverJaegerTChannelPort = v.GetInt(receiverJaegerTChannelPort)
	rOpts.ReceiverJaegerHTTPPort = v.GetInt(receiverJaegerHTTPPort)
	rOpts.ReceiverZipkinHTTPPort = v.GetInt(receiverZipkinHTTPort)
	return rOpts
}

// InitFromViper initializes ReporterBuilder with properties from viper
func (rOpts *ReporterOptions) InitFromViper(v *viper.Viper) *ReporterOptions {
	if len(v.GetString(reporterTChannelHostPort)) > 0 {
		rOpts.ReporterTChannelHostPorts = strings.Split(v.GetString(reporterTChannelHostPort), ",")
	}
	rOpts.ReporterTChannelDiscoveryMinPeers = v.GetInt(reporterTChannelDiscoveryMinPeers)
	rOpts.ReporterTChannelDiscoveryConnCheckTimeout = v.GetDuration(reporterTChannelDiscoveryConnCheckTimeout)
	return rOpts
}

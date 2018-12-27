package http

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	httpPrefix        = "reporter.http."
	collectorEndpoint = httpPrefix + "collector-endpoint"
	requestTimeout    = httpPrefix + "request-timeout"
)

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(
		collectorEndpoint,
		"",
		"Comma-separated string reprensenting HTTP endpoints of a static list of collectors to which the Span will be sent.")
	flags.Duration(
		requestTimeout,
		defaultRequestTimeout,
		"Sets the timeout in SECONDs used when sending spans to collector, default to 5s")
}

// InitFromViper initializes Options with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) *Builder {
	b.WithCollectorEndpoint(v.GetString(collectorEndpoint))
	b.WithRequestTimeout(v.GetDuration(requestTimeout))
	return b
}

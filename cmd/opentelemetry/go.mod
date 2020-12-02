module github.com/jaegertracing/jaeger/cmd/opentelemetry

go 1.14

replace github.com/jaegertracing/jaeger => ./../../

require (
	github.com/Shopify/sarama v1.27.2
	github.com/elastic/go-elasticsearch/v6 v6.8.10
	github.com/elastic/go-elasticsearch/v7 v7.0.0
	github.com/imdario/mergo v0.3.9
	github.com/jaegertracing/jaeger v1.21.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible
	go.opencensus.io v0.22.5
	go.opentelemetry.io/collector v0.16.0
	go.uber.org/zap v1.16.0
)

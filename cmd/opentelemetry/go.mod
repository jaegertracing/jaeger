module github.com/jaegertracing/jaeger/cmd/opentelemetry

go 1.14

replace github.com/jaegertracing/jaeger => ./../../

require (
	github.com/Shopify/sarama v1.27.0
	github.com/elastic/go-elasticsearch/v6 v6.8.10
	github.com/elastic/go-elasticsearch/v7 v7.0.0
	github.com/imdario/mergo v0.3.9
	github.com/jaegertracing/jaeger v1.18.2-0.20200707061226-97d2319ff2be
	github.com/olivere/elastic v6.2.27+incompatible
	github.com/opentracing/opentracing-go v1.1.1-0.20190913142402-a7454ce5950e
	github.com/shirou/gopsutil v2.20.4+incompatible // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	github.com/uber/jaeger-client-go v2.23.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	go.opentelemetry.io/collector v0.8.1-0.20200820012544-1e65674799c8
	go.uber.org/zap v1.15.0
)

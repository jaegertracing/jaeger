module github.com/jaegertracing/jaeger/cmd/opentelemetry-collector

go 1.13

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/jaegertracing/jaeger => ./../../

require (
	github.com/Shopify/sarama v1.22.2-0.20190604114437-cd910a683f9f
	github.com/imdario/mergo v0.3.9
	github.com/jaegertracing/jaeger v1.17.0
	github.com/magiconair/properties v1.8.1
	github.com/open-telemetry/opentelemetry-collector v0.3.1-0.20200503151053-5d1aacc0e168
	github.com/open-telemetry/opentelemetry-proto v0.3.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	github.com/stretchr/testify v1.5.1
	github.com/uber/jaeger-lib v2.2.0+incompatible
	go.uber.org/zap v1.13.0
	k8s.io/client-go v12.0.0+incompatible // indirect
)

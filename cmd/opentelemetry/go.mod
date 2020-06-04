module github.com/jaegertracing/jaeger/cmd/opentelemetry

go 1.13

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/jaegertracing/jaeger => ./../../

require (
	github.com/Shopify/sarama v1.22.2-0.20190604114437-cd910a683f9f
	github.com/imdario/mergo v0.3.9
	github.com/jaegertracing/jaeger v1.17.0
	github.com/opentracing/opentracing-go v1.1.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	github.com/stretchr/testify v1.5.1
	github.com/uber/jaeger-client-go v2.22.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	go.opentelemetry.io/collector v0.3.1-0.20200601172059-a776048b653c
	go.uber.org/zap v1.13.0
	k8s.io/client-go v12.0.0+incompatible // indirect
)

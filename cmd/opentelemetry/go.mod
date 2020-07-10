module github.com/jaegertracing/jaeger/cmd/opentelemetry

go 1.14

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/jaegertracing/jaeger => ./../../

require (
	github.com/Shopify/sarama v1.22.2-0.20190604114437-cd910a683f9f
	github.com/elastic/go-elasticsearch/v6 v6.8.10
	github.com/elastic/go-elasticsearch/v7 v7.0.0
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/google/go-cmp v0.5.0 // indirect
	github.com/imdario/mergo v0.3.9
	github.com/jaegertracing/jaeger v1.18.2-0.20200707061226-97d2319ff2be
	github.com/olivere/elastic v6.2.27+incompatible
	github.com/opentracing/opentracing-go v1.1.1-0.20190913142402-a7454ce5950e
	github.com/shirou/gopsutil v2.20.4+incompatible // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.6.1
	github.com/uber/jaeger-client-go v2.22.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	go.opentelemetry.io/collector v0.5.1-0.20200708032135-c966e140fd4f
	go.uber.org/zap v1.13.0
	google.golang.org/genproto v0.0.0-20200624020401-64a14ca9d1ad // indirect
	k8s.io/client-go v12.0.0+incompatible // indirect
)

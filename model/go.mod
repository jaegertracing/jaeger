module github.com/jaegertracing/jaeger/model

go 1.21.6

replace github.com/jaegertracing/jaeger => ../

require (
	github.com/apache/thrift v0.19.0
	github.com/gogo/protobuf v1.3.2
	github.com/jaegertracing/jaeger v0.0.0-00010101000000-000000000000
	github.com/kr/pretty v0.3.1
	github.com/stretchr/testify v1.8.4
	go.opentelemetry.io/otel v1.22.0
	go.opentelemetry.io/otel/trace v1.22.0
	go.uber.org/zap v1.26.0
	google.golang.org/protobuf v1.32.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/grpc v1.61.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

module github.com/jaegertracing/jaeger/cmd/opentelemetry-collector

go 1.13

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/jaegertracing/jaeger => ./../../

require github.com/open-telemetry/opentelemetry-collector v0.2.8-0.20200318211436-c7a11d6181c1 // indirect

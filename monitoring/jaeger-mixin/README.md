# Prometheus monitoring mixin for Jaeger

The Prometheus monitoring mixin for Jaeger provides a starting point for people wanting to monitor Jaeger using Prometheus, Alertmanager, and Grafana. To use it, you'll need [`jsonnet`](https://github.com/google/go-jsonnet) and [`jb` (jsonnet-bundler)](https://github.com/jsonnet-bundler/jsonnet-bundler). They can be installed using `go get`, as follows:

```console
$ go get github.com/google/go-jsonnet/cmd/jsonnet
$ go get github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb
```

Your monitoring mixin can then be initialized as follows:

```console
$ jb init
$ jb install \
  github.com/jaegertracing/jaeger/monitoring/jaeger-mixin@master \
  github.com/grafana/jsonnet-libs/grafana-builder@master \
  github.com/coreos/kube-prometheus/jsonnet/kube-prometheus@master
```

Your mixin can then look like this:

```jsonnet
local jaegerAlerts = (import 'jaeger-mixin/alerts.libsonnet').prometheusAlerts;
local jaegerDashboard = (import 'jaeger-mixin/mixin.libsonnet').grafanaDashboards;

local kp =
  (import 'kube-prometheus/kube-prometheus.libsonnet') +
  {
    _config+:: {
      namespace: 'monitoring',
    },
    grafanaDashboards+:: {
      'jaeger.json': jaegerDashboard['jaeger.json'],
    },
    prometheusAlerts+:: jaegerAlerts,
  };

{ ['00namespace-' + name + '.json']: kp.kubePrometheus[name] for name in std.objectFields(kp.kubePrometheus) } +
{ ['0prometheus-operator-' + name + '.json']: kp.prometheusOperator[name] for name in std.objectFields(kp.prometheusOperator) } +
{ ['node-exporter-' + name + '.json']: kp.nodeExporter[name] for name in std.objectFields(kp.nodeExporter) } +
{ ['kube-state-metrics-' + name + '.json']: kp.kubeStateMetrics[name] for name in std.objectFields(kp.kubeStateMetrics) } +
{ ['alertmanager-' + name + '.json']: kp.alertmanager[name] for name in std.objectFields(kp.alertmanager) } +
{ ['prometheus-' + name + '.json']: kp.prometheus[name] for name in std.objectFields(kp.prometheus) } +
{ ['prometheus-adapter-' + name + '.json']: kp.prometheusAdapter[name] for name in std.objectFields(kp.prometheusAdapter) } +
{ ['grafana-' + name + '.json']: kp.grafana[name] for name in std.objectFields(kp.grafana) }
```

The manifest files can be generated via `jsonnet` and passed directly to `kubectl`:

```console
$ jsonnet -J vendor -cm manifests/ monitoring-setup.jsonnet
$ kubectl apply -f manifests/
```

Make sure your Prometheus setup is properly scraping the Jaeger components, either by creating a `ServiceMonitor` (and the backing `Service` objects), or via `PodMonitor` resources, like:

```console
$ kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: tracing
  namespace: monitoring
spec:
  podMetricsEndpoints:
  - interval: 5s
    targetPort: 14269
  selector:
    matchLabels:
      app: jaeger
EOF
```

This `PodMonitor` tells Prometheus to scrape the port `14269` from all pods containing the label `app: jaeger`. If you have the Jaeger Collector, Agent, and Query in different pods, you might need to adjust or create further `PodMonitor` resources to scrape metrics from the other ports.

This mixin was originally developed by [Grafana Labs](https://github.com/grafana/jsonnet-libs/tree/master/jaeger-mixin).

## Background

* For more information about monitoring mixins, see this [design doc](https://docs.google.com/document/d/1A9xvzwqnFVSOZ5fD3blKODXfsat5fg6ZhnKu9LK3lB4/edit#).

# Prometheus monitoring mixin for Jaeger

The Prometheus monitoring mixin for Jaeger provides a starting point for people wanting to monitor Jaeger using Prometheus, Alertmanager, and Grafana. To use it, you'll need [`jsonnet`](https://github.com/google/go-jsonnet) and [`jb` (jsonnet-bundler)](https://github.com/jsonnet-bundler/jsonnet-bundler). They can be installed using `go get`, as follows:

```console
mkdir -p ~/bin && curl -sL https://github.com/google/go-jsonnet/releases/download/v0.17.0/go-jsonnet_0.17.0_Linux_x86_64.tar.gz | tar -C ~/bin/ -xzf -
curl -sLo ~/bin/jb https://github.com/jsonnet-bundler/jsonnet-bundler/releases/download/v0.4.0/jb-linux-amd64
chmod +x ~/bin/jb
```

Your monitoring mixin can then be initialized as follows:

```console
jb init
jb install \
  github.com/jaegertracing/jaeger/monitoring/jaeger-mixin@main \
  github.com/grafana/jsonnet-libs/grafana-builder@master \
  github.com/coreos/kube-prometheus/jsonnet/kube-prometheus@main
```

In the directory where your mixin was initialized, create a new `monitoring-setup.jsonnet`, specifying how your monitoring stack should look like: this file is yours, any customizations to Prometheus, Grafana, or Alertmanager should take place here. A simple example providing only the Jaeger dashboard for Grafana would be:

```jsonnet
local jaegerDashboard = (import 'jaeger-mixin/mixin.libsonnet').grafanaDashboards;
{ ['dashboards-jaeger.json']: jaegerDashboard['jaeger.json'] }
```

The manifest files can be generated via the `jsonnet` command below. Once the command finishes, the file `manifests/dashboards-jaeger.json` should be available and can be loaded directly into Grafana.

```console
jsonnet -J vendor -cm manifests/ monitoring-setup.jsonnet
```

An example producing the manifests for a complete monitoring stack is located in this directory, as `monitoring-setup.example.jsonnet`. The manifests include Prometheus, Grafana, and Alertmanager managed via the Prometheus Operator for Kubernetes.

```jsonnet
local jaegerAlerts = (import 'jaeger-mixin/alerts.libsonnet').prometheusAlerts;
local jaegerDashboard = (import 'jaeger-mixin/mixin.libsonnet').grafanaDashboards;

local kp =
  (import 'kube-prometheus/main.libsonnet') +
  {
    values+:: {
      common+: {
        namespace: 'observability',
      },
      grafana+: {
        dashboards+:: {
          'my-dashboard.json': jaegerDashboard['jaeger.json'],
        },
      },
    },
    exampleApplication: {
      prometheusRuleExample: {
        apiVersion: 'monitoring.coreos.com/v1',
        kind: 'PrometheusRule',
        metadata: {
          name: 'my-prometheus-rule',
          namespace: $.values.common.namespace,
        },
        spec: jaegerAlerts,
      },
    },
  };
{ ['00namespace-' + name + '.json']: kp.kubePrometheus[name] for name in std.objectFields(kp.kubePrometheus) } +
{ ['0prometheus-operator-' + name + '.json']: kp.prometheusOperator[name] for name in std.objectFields(kp.prometheusOperator) } +
{ ['node-exporter-' + name + '.json']: kp.nodeExporter[name] for name in std.objectFields(kp.nodeExporter) } +
{ ['kube-state-metrics-' + name + '.json']: kp.kubeStateMetrics[name] for name in std.objectFields(kp.kubeStateMetrics) } +
{ ['alertmanager-' + name + '.json']: kp.alertmanager[name] for name in std.objectFields(kp.alertmanager) } +
{ ['prometheus-' + name + '.json']: kp.prometheus[name] for name in std.objectFields(kp.prometheus) } +
{ ['prometheus-adapter-' + name + '.json']: kp.prometheusAdapter[name] for name in std.objectFields(kp.prometheusAdapter) } +
{ ['grafana-' + name + '.json']: kp.grafana[name] for name in std.objectFields(kp.grafana) } +
{ ['my-application-' + name + '.json']: kp.exampleApplication[name] for name in std.objectFields(kp.exampleApplication) }
```

The manifest files can be generated via `jsonnet` and passed directly to `kubectl`:

```console
jsonnet -J vendor -cm manifests/ monitoring-setup.jsonnet
kubectl apply -f manifests/
```

The resulting manifests will include everything that is needed to have a Prometheus, Alertmanager, and Grafana instances. Whenever a new alert rule is needed, or a new dashboard has to be defined, change your `monitoring-setup.jsonnet`, re-generate and re-apply the manifests.

Make sure your Prometheus setup is properly scraping the Jaeger components, either by creating a `ServiceMonitor` (and the backing `Service` objects), or via `PodMonitor` resources, like:

```console
kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: tracing
  namespace: observability
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

## Pre-built dashboard and alert rules

This repository contains also a pre-built dashboard for Grafana and alert rules for Alertmanager. While we recommend that you generate those resources following the steps above, the following files can be used for quick tests.

- [Dashboard](./dashboard-for-grafana.json)
- [Alerts](./prometheus_alerts.yml)

_IMPORTANT_: the metrics that are used by default by the dashboard are compatible with the components deployed as part of the production strategy, where each component is deployed individually. Some metric names differ from the ones used in the all-in-one strategy. Adjust your dashboard to reflect your scenario.

## Background

* For more information about monitoring mixins, see this [design doc](https://docs.google.com/document/d/1A9xvzwqnFVSOZ5fD3blKODXfsat5fg6ZhnKu9LK3lB4/view).

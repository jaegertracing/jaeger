# Prometheus monitoring mixin for Jaeger

The Prometheus monitoring mixin for Jaeger provides a starting point for people wanting to monitor Jaeger using Prometheus, Alertmanager, and Grafana.

The dashboard in this directory is committed as [dashboard-for-grafana.json](./dashboard-for-grafana.json) and generated from the Go source in `generate/` using `grafana-foundation-sdk/go`. If you only need the dashboard, you can import that JSON directly into Grafana or mount it into a provisioning directory. To regenerate it after editing the Go source, run:

```console
make generate-dashboards
```

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

## Pre-built dashboard

This repository contains a committed Grafana dashboard:

- [Dashboard](./dashboard-for-grafana.json)

_IMPORTANT_: the metrics that are used by default by the dashboard are compatible with the components deployed as part of the production strategy, where each component is deployed individually. Some metric names differ from the ones used in the all-in-one strategy. Adjust your dashboard to reflect your scenario.

## Alert rules

This mixin no longer ships built-in alert rules. The previously bundled `prometheus_alerts.yml` contained Jaeger v1 alerts that relied on metrics no longer emitted by Jaeger v2.

Since Jaeger v2 uses standard OpenTelemetry Collector metrics, any OTEL Collector alerting rules apply directly. For a maintained example of Prometheus alert rules for the OpenTelemetry Collector, see the [opentelemetry-helm-charts PrometheusRule template](https://github.com/open-telemetry/opentelemetry-helm-charts/blob/main/charts/opentelemetry-collector/templates/prometheusrule.yaml).

## Background

* For background and historical context on the monitoring mixin, see [ADR-007](https://github.com/jaegertracing/jaeger/blob/main/docs/adr/007-grafana-dashboards-modernization.md). Note that the ADR describes an earlier Jsonnet-based approach and different filenames; the current implementation is the Go-based mixin and `dashboard-for-grafana.json` described above.

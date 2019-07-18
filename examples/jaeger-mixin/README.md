# Prometheus monitoring mixin for Jaeger

A set of customisable alerts and dashboards for Jaeger. 

Instructions for use are the same as the [kubernetes-mixin](https://github.com/kubernetes-monitoring/kubernetes-mixin).

Originally developed by [Grafana Labs](https://github.com/grafana/jsonnet-libs/tree/master/jaeger-mixin).

## Background

* For more information about monitoring mixins, see this [design doc](https://docs.google.com/document/d/1A9xvzwqnFVSOZ5fD3blKODXfsat5fg6ZhnKu9LK3lB4/edit#).

## Outputting alerts and dashboards 

Make sure to have [jsonnet](https://jsonnet.org/), [gojsontoyaml](https://github.com/brancz/gojsontoyaml) and [jsonnet-bundler](https://github.com/jsonnet-bundler/) installed.

Compile the mixin to a Prometheus alerts YAML file:
```
jsonnet -e '(import "mixin.libsonnet").prometheusAlerts' | gojsontoyaml > alerts.yaml
```

Compile the dashboards, first get the jsonnet dependencies and then compile:
```
jb install
jsonnet -J vendor -m .  -e '(import "mixin.libsonnet").grafanaDashboards'
```


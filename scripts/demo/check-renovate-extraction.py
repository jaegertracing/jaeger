#!/usr/bin/env python3

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Regression check against Renovate's actual extraction, not its regex syntax."""

import json
import sys


expected = {
    ("examples/oci/deploy-all.sh", "kube-prometheus-stack"),
    ("examples/oci/jaeger-values.yaml", "jaegertracing/example-hotrod"),
    ("examples/oci/load-generator/load-generator.yaml", "python"),
    ("examples/oci/load-generator/requirements.txt", "requests"),
    ("examples/otel-demo/deploy-all.sh", "opensearch"),
    ("examples/otel-demo/deploy-all.sh", "opensearch-dashboards"),
    ("examples/otel-demo/deploy-all.sh", "opentelemetry-demo"),
    ("examples/otel-demo/jaeger-values.yaml", "jaegertracing/jaeger-es-index-cleaner"),
    ("examples/otel-demo/otel-demo-values.yaml", "docker.io/busybox"),
    ("examples/otel-demo/otel-demo-values.yaml", "docker.io/valkey/valkey"),
    ("examples/otel-demo/otel-demo-values.yaml", "docker.io/library/postgres"),
    ("examples/otel-demo/load-generator.yaml", "python"),
    ("examples/otel-demo/load-generator/requirements.txt", "requests"),
    ("scripts/demo/jaeger-chart.sh", "https://github.com/jaegertracing/helm-charts.git"),
}
found = set()
with open(sys.argv[1], encoding="utf-8") as source:
    for line in source:
        entry = json.loads(line)
        if entry.get("msg") == "Extracted dependencies":
            for files in entry["packageFiles"].values():
                for package in files:
                    found.update((package["packageFile"], dep["depName"]) for dep in package["deps"])
missing = expected - found
if missing:
    raise SystemExit("Renovate did not discover hosted demo dependencies: " + repr(sorted(missing)))
print(f"Renovate discovered all {len(expected)} required hosted demo dependency locations")

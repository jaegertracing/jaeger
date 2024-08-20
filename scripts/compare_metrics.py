# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json

v1_metrics_path = "./V1_Metrics.json"
v2_metrics_path = "./V2_Metrics.json"

with open(v1_metrics_path, 'r') as file:
    v1_metrics = json.load(file)

with open(v2_metrics_path, 'r') as file:
    v2_metrics = json.load(file)

# Extract names and labels of the metrics
def extract_metrics_with_labels(metrics, strip_prefix=None):
    result = {}
    for metric in metrics:
        name = metric['name']
        if strip_prefix and name.startswith(strip_prefix):
            name = name[len(strip_prefix):]
        labels = {}
        if 'metrics' in metric and 'labels' in metric['metrics'][0]:
            labels = metric['metrics'][0]['labels']
        result[name] = labels
    return result


v1_metrics_with_labels = extract_metrics_with_labels(v1_metrics)
v2_metrics_with_labels = extract_metrics_with_labels(
    v2_metrics, strip_prefix="otelcol_")

# Compare the metrics names and labels
common_metrics = {}
v1_only_metrics = {}
v2_only_metrics = {}

for name, labels in v1_metrics_with_labels.items():
    if name in v2_metrics_with_labels:
        common_metrics[name] = labels
    elif not name.startswith("jaeger_agent"):
        v1_only_metrics[name] = labels

for name, labels in v2_metrics_with_labels.items():
    if name not in v1_metrics_with_labels:
        v2_only_metrics[name] = labels

differences = {
    "common_metrics": common_metrics,
    "v1_only_metrics": v1_only_metrics,
    "v2_only_metrics": v2_only_metrics
}

# Write the differences to a new JSON file
differences_path = "./differences.json"
with open(differences_path, 'w') as file:
    json.dump(differences, file, indent=4)

print(f"Differences written to {differences_path}")

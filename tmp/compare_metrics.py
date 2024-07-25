# Run the following commands first to create the JSON files:
# Run V1 Binary
# prom2json http://localhost:14269/metrics > V1_Metrics.json
# Run V2 Binary
# prom2json http://localhost:8888/metrics > V2_Metrics.json

import json

# Load the JSON files
v1_metrics_path = "./V1_Metrics.json"
v2_metrics_path = "./V2_Metrics.json"

with open(v1_metrics_path, 'r') as file:
    v1_metrics = json.load(file)

with open(v2_metrics_path, 'r') as file:
    v2_metrics = json.load(file)

# Extract names and labels of the metrics
def extract_metrics_with_labels(metrics):
    result = {}
    for metric in metrics:
        name = metric['name']
        labels = {}
        if 'metrics' in metric and 'labels' in metric['metrics'][0]:
            labels = metric['metrics'][0]['labels']
        result[name] = labels
    return result

v1_metrics_with_labels = extract_metrics_with_labels(v1_metrics)
v2_metrics_with_labels = extract_metrics_with_labels(v2_metrics)

# Compare the metrics names and labels
common_metrics = {}
v1_only_metrics = {}
v2_only_metrics = {}

for name, labels in v1_metrics_with_labels.items():
    if name in v2_metrics_with_labels:
        if labels == v2_metrics_with_labels[name]:
            common_metrics[name] = labels
        else:
            v1_only_metrics[name] = labels
    else:
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

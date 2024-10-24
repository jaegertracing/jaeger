# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
import argparse
import subprocess

#Instructions of use:

# To generate V1_Metrics.json and V2_Metrics.json, run the following commands:
# i.e for elastic search first run the following command:
# docker compose -f docker-compose/elasticsearch/v7/docker-compose.yml up
# 1. Generate V1_Metrics.json and V2_Metrics.json by the following commands:
# V1 binary cmd: SPAN_STORAGE_TYPE=elasticsearch go run -tags=ui ./cmd/all-in-one
# extract the metrics by running the following command:
# prom2json http://localhost:14269/metrics > V1_Metrics.json
# Stop the v1 binary and for v2 binary run the following command:
# go run -tags ui ./cmd/jaeger/main.go --config ./cmd/jaeger/config-elasticsearch.yaml
# extract the metrics by running the following command:
# prom2json http://localhost:8888/metrics > V2_Metrics.json
# it is first recomended to generate the differences for all-in-one.json by running the following command:
# python3 compare_metrics.py --out md --is_storage F
# rename that file to all_in_one.json and use it to filter out the overlapping metrics by using the is_storage falg to T
# 2. Run the script with the following command:
#    python3 compare_metrics.py --out {json or md} --is_storage {T or F}
# 3. The script will compare the metrics in V1_Metrics.json and V2_Metrics.json and output the differences to differences.json


# Extract names and labels of the metrics
def extract_metrics_with_labels(metrics, strip_prefix=None):
    result = {}
    for metric in metrics:
        
        name = metric['name']
        print(name)
        if strip_prefix and name.startswith(strip_prefix):
            name = name[len(strip_prefix):]
        labels = {}
        if 'metrics' in metric and 'labels' in metric['metrics'][0]:
            labels = metric['metrics'][0]['labels']
        result[name] = labels
    return result

def remove_overlapping_metrics(all_in_one_data, other_json_data):
    """Remove overlapping metrics found in all_in-one.json from another JSON."""
    # Loop through v1 and v2 metrics to remove overlaps
    for metric_category in ['common_metrics', 'v1_only_metrics', 'v2_only_metrics']:
        if metric_category in all_in_one_data and metric_category in other_json_data:
            for metric in all_in_one_data[metric_category]:
                if metric in other_json_data[metric_category]:
                    del other_json_data[metric_category][metric]

    return other_json_data


# Your current compare_metrics.py logic goes here
def main():
    parser = argparse.ArgumentParser(description='Compare metrics and output format.')
    parser.add_argument('--out', choices=['json', 'md'], default='json',
                        help='Output format: json (default) or md')
    parser.add_argument('--is_storage', choices=['T','F'],default='F', help='Remove overlapping storage metrics')
    # Parse the arguments
    args = parser.parse_args()

    # Call your existing compare logic here
    print("Running metric comparison...")
    v1_metrics_path = "" #Add the path to the V1_Metrics.json file
    v2_metrics_path = "" #Add the path to the V2_Metrics.json file      

    with open(v1_metrics_path, 'r') as file:
       v1_metrics = json.load(file)

    with open(v2_metrics_path, 'r') as file:
       v2_metrics = json.load(file)

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
      "v2_only_metrics": v2_only_metrics,
    }

    #Write the differences to a new JSON file
    differences_path = "./differences.json"
    with open(differences_path, 'w') as file:
       json.dump(differences, file, indent=4)

    print(f"Differences written to {differences_path}")
    if args.is_storage == 'T':
        all_in_one_path = "" #Add the path to the all_in_one.json file
        with open(all_in_one_path, 'r') as file:
            all_in_one_data = json.load(file)
        with open(differences_path, 'r') as file:
            other_json_data = json.load(file)
        other_json_data = remove_overlapping_metrics(all_in_one_data, other_json_data)
        with open(differences_path, 'w') as file:
            json.dump(other_json_data, file, indent=4)
        print(f"Overlapping storage metrics removed from {differences_path}")
    # If the user requested markdown output, run metrics_md.py
    if args.out == 'md':
        try:
            print("Running metrics_md.py to generate markdown output...")
            subprocess.run(['python3', 'metrics-md.py'], check=True)
        except subprocess.CalledProcessError as e:
            print(f"Error running metrics_md.py: {e}")
    
    # If json output is requested or no output type is provided (default is json)
    else:
        print("Output in JSON format.")

if __name__ == "__main__":
    main()
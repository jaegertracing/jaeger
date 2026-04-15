# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Export Prometheus metrics snapshots to a structured YAML data file.

This script reads raw Prometheus text-format snapshot files (as scraped from
Jaeger's /metrics endpoint by the integration tests) and produces a single
YAML file suitable for consumption by the documentation website.

The output follows a similar pattern to the CLI flags YAML files stored in
``data/cli/{version}/`` in the jaegertracing/documentation repository.  The
documentation site can place this output in ``data/metrics/{version}/`` and
render it with a Hugo template, keeping all styling and layout decisions in
the template layer rather than generating HTML or Markdown directly.

Usage::

    python3 scripts/e2e/export_metrics_to_yaml.py \\
        --snapshot-dir .metrics \\
        --output metrics.yaml

The ``--snapshot-dir`` should contain one or more ``metrics_snapshot_*.txt``
files produced by the E2E integration tests.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from collections import defaultdict
from typing import Any

# PyYAML is preferred for output (human-readable, block style).
# prometheus_client is used for parsing the Prometheus text format.
try:
    import yaml
except ImportError:
    yaml = None  # type: ignore[assignment]

try:
    from prometheus_client.parser import text_string_to_metric_families
except ImportError:
    text_string_to_metric_families = None  # type: ignore[assignment]


# Labels that carry per-instance identity and should be stripped from the
# documentation output (they add noise without informational value).
_EXCLUDED_LABELS: frozenset[str] = frozenset({
    "service_instance_id",
    "otel_scope_version",
    "otel_scope_schema_url",
})


def parse_snapshot(content: str) -> list[dict[str, Any]]:
    """Parse a Prometheus text-format snapshot into a list of metric dicts.

    Each dict has the following structure::

        {
            "name": "http_server_duration_milliseconds_bucket",
            "type": "histogram",     # counter | gauge | histogram | summary | untyped
            "help": "Duration of HTTP server requests.",
            "labels": ["le", "http_method", "http_route"],
        }

    Metrics are deduplicated by name: the ``labels`` list is the union of all
    label keys seen across all samples of that metric.  Label *values* are
    intentionally omitted -- the documentation page shows which metrics exist
    and what dimensions they carry, not the actual runtime values.
    """
    if text_string_to_metric_families is None:
        raise RuntimeError(
            "prometheus_client is required: pip install prometheus-client"
        )

    metrics_map: dict[str, dict[str, Any]] = {}

    for family in text_string_to_metric_families(content):
        if family.name in metrics_map:
            entry = metrics_map[family.name]
        else:
            entry = {
                "name": family.name,
                "type": family.type,
                "help": family.documentation or "",
                "labels": set(),
            }
            metrics_map[family.name] = entry

        for sample in family.samples:
            for label_key in sample.labels:
                if label_key not in _EXCLUDED_LABELS:
                    entry["labels"].add(label_key)

    # Convert label sets to sorted lists for deterministic output.
    result: list[dict[str, Any]] = []
    for entry in metrics_map.values():
        result.append({
            "name": entry["name"],
            "type": entry["type"],
            "help": entry["help"],
            "labels": sorted(entry["labels"]),
        })

    result.sort(key=lambda m: m["name"])
    return result


def collect_snapshots(
    snapshot_dir: str,
) -> dict[str, list[dict[str, Any]]]:
    """Read all ``metrics_snapshot_*.txt`` files in *snapshot_dir*.

    Supports two directory layouts:

    1. **Flat**: snapshot files directly in *snapshot_dir*::

           snapshot_dir/metrics_snapshot_memory.txt

    2. **Artifact subdirectories** (as produced by ``gh run download``)::

           snapshot_dir/metrics_snapshot_memory/metrics_snapshot_memory.txt

    Returns a dict mapping the backend name (extracted from the filename,
    e.g. ``"memory"``, ``"elasticsearch"``) to the parsed metric list.
    When duplicate backend names are found (e.g. from matrix variations),
    the metrics are merged.
    """
    snapshots: dict[str, list[dict[str, Any]]] = {}
    file_pattern = re.compile(r"^metrics_snapshot_(.+)\.txt$")

    if not os.path.isdir(snapshot_dir):
        print(f"Warning: snapshot directory does not exist: {snapshot_dir}",
              file=sys.stderr)
        return snapshots

    snapshot_files: list[tuple[str, str]] = []  # (backend_name, filepath)

    for entry in sorted(os.listdir(snapshot_dir)):
        entry_path = os.path.join(snapshot_dir, entry)

        # Case 1: file directly in snapshot_dir
        if os.path.isfile(entry_path):
            match = file_pattern.match(entry)
            if match:
                snapshot_files.append((match.group(1), entry_path))

        # Case 2: subdirectory containing the snapshot file
        elif os.path.isdir(entry_path):
            for sub_entry in sorted(os.listdir(entry_path)):
                match = file_pattern.match(sub_entry)
                if match:
                    sub_path = os.path.join(entry_path, sub_entry)
                    if os.path.isfile(sub_path):
                        snapshot_files.append((match.group(1), sub_path))

    for backend, filepath in snapshot_files:
        with open(filepath, "r") as f:
            content = f.read()
        parsed = parse_snapshot(content)
        if backend in snapshots:
            # Merge with existing: union of metrics
            existing_names = {m["name"] for m in snapshots[backend]}
            for metric in parsed:
                if metric["name"] not in existing_names:
                    snapshots[backend].append(metric)
                    existing_names.add(metric["name"])
            snapshots[backend].sort(key=lambda m: m["name"])
        else:
            snapshots[backend] = parsed

    return snapshots


def merge_snapshots(
    snapshots: dict[str, list[dict[str, Any]]],
) -> list[dict[str, Any]]:
    """Merge metrics from multiple backend snapshots into a unified list.

    Because different backends exercise different code paths, the full set of
    metrics is only available when all snapshots are combined.  Merging
    unions the label sets and keeps the help/type from the first occurrence.

    An extra field ``"sources"`` lists the backend names where each metric
    was observed.
    """
    merged: dict[str, dict[str, Any]] = {}

    for backend, metrics in sorted(snapshots.items()):
        for metric in metrics:
            name = metric["name"]
            if name in merged:
                existing = merged[name]
                label_set = set(existing["labels"])
                label_set.update(metric["labels"])
                existing["labels"] = sorted(label_set)
                if backend not in existing["sources"]:
                    existing["sources"].append(backend)
            else:
                merged[name] = {
                    "name": metric["name"],
                    "type": metric["type"],
                    "help": metric["help"],
                    "labels": list(metric["labels"]),
                    "sources": [backend],
                }

    result = sorted(merged.values(), key=lambda m: m["name"])
    return result


def build_yaml_output(
    merged_metrics: list[dict[str, Any]],
    per_backend: dict[str, list[dict[str, Any]]],
) -> dict[str, Any]:
    """Build the top-level YAML structure.

    The output has two sections:

    - ``metrics``: the merged list of all metrics across all backends.
    - ``backends``: a mapping from backend name to its metric list,
      useful for backend-specific documentation pages.

    Each metric entry has:

    - ``name``: Prometheus metric name.
    - ``type``: Prometheus metric type (counter, gauge, histogram, etc.).
    - ``help``: the HELP string from the Prometheus exposition.
    - ``labels``: sorted list of label keys (excluding instance-specific ones).
    - ``sources`` (merged only): list of backend names where this metric
      was observed.
    """
    backends_summary: dict[str, dict[str, Any]] = {}
    for backend_name in sorted(per_backend.keys()):
        backend_metrics = per_backend[backend_name]
        backends_summary[backend_name] = {
            "count": len(backend_metrics),
            "metrics": backend_metrics,
        }

    return {
        "total_metrics": len(merged_metrics),
        "total_backends": len(per_backend),
        "metrics": merged_metrics,
        "backends": backends_summary,
    }


def write_yaml(data: dict[str, Any], output_path: str) -> None:
    """Write data to a YAML file."""
    if yaml is None:
        raise RuntimeError("PyYAML is required: pip install pyyaml")

    with open(output_path, "w") as f:
        yaml.dump(
            data,
            f,
            default_flow_style=False,
            sort_keys=False,
            allow_unicode=True,
        )
    print(f"Metrics YAML written to {output_path}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Export Prometheus metrics snapshots to YAML for documentation"
    )
    parser.add_argument(
        "--snapshot-dir",
        required=True,
        help="Directory containing metrics_snapshot_*.txt files",
    )
    parser.add_argument(
        "--output",
        "-o",
        default="metrics.yaml",
        help="Output YAML file path (default: metrics.yaml)",
    )

    args = parser.parse_args(argv)

    snapshots = collect_snapshots(args.snapshot_dir)
    if not snapshots:
        print("No metrics snapshot files found.", file=sys.stderr)
        return 1

    merged = merge_snapshots(snapshots)
    output_data = build_yaml_output(merged, snapshots)
    write_yaml(output_data, args.output)

    print(
        f"Exported {output_data['total_metrics']} unique metrics "
        f"from {output_data['total_backends']} backend(s)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())

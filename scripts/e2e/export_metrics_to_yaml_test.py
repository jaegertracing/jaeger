# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import os
import tempfile
import unittest

import yaml

from export_metrics_to_yaml import (
    build_yaml_output,
    collect_snapshots,
    merge_snapshots,
    parse_snapshot,
)

# Minimal Prometheus text-format snippets used across tests.
_SNAPSHOT_MEMORY = """\
# HELP http_server_duration_milliseconds Duration of HTTP server requests.
# TYPE http_server_duration_milliseconds histogram
http_server_duration_milliseconds_bucket{http_method="GET",http_route="/api/traces",le="100"} 5
http_server_duration_milliseconds_bucket{http_method="GET",http_route="/api/traces",le="+Inf"} 10
http_server_duration_milliseconds_count{http_method="GET",http_route="/api/traces"} 10
http_server_duration_milliseconds_sum{http_method="GET",http_route="/api/traces"} 450
# HELP jaeger_storage_spans_written_total Total number of spans written to storage.
# TYPE jaeger_storage_spans_written_total counter
jaeger_storage_spans_written_total{storage="memory"} 42
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 1.23
"""

_SNAPSHOT_CASSANDRA = """\
# HELP http_server_duration_milliseconds Duration of HTTP server requests.
# TYPE http_server_duration_milliseconds histogram
http_server_duration_milliseconds_bucket{http_method="POST",http_route="/api/traces",le="100"} 3
http_server_duration_milliseconds_bucket{http_method="POST",http_route="/api/traces",le="+Inf"} 8
http_server_duration_milliseconds_count{http_method="POST",http_route="/api/traces"} 8
http_server_duration_milliseconds_sum{http_method="POST",http_route="/api/traces"} 320
# HELP jaeger_storage_spans_written_total Total number of spans written to storage.
# TYPE jaeger_storage_spans_written_total counter
jaeger_storage_spans_written_total{storage="cassandra"} 100
# HELP jaeger_storage_cassandra_errors_total Total number of Cassandra errors.
# TYPE jaeger_storage_cassandra_errors_total counter
jaeger_storage_cassandra_errors_total{operation="write"} 0
"""

_SNAPSHOT_WITH_EXCLUDED_LABELS = """\
# HELP my_metric A test metric.
# TYPE my_metric gauge
my_metric{job="jaeger",service_instance_id="abc-123",otel_scope_version="1.0"} 1
"""


class TestParseSnapshot(unittest.TestCase):
    """Tests for parse_snapshot()."""

    def test_parses_metric_names(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        names = [m["name"] for m in metrics]
        self.assertIn("http_server_duration_milliseconds", names)
        # prometheus_client parser uses family.name which strips _total suffix
        self.assertIn("jaeger_storage_spans_written", names)
        self.assertIn("process_cpu_seconds", names)

    def test_parses_metric_types(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        by_name = {m["name"]: m for m in metrics}
        self.assertEqual(by_name["http_server_duration_milliseconds"]["type"], "histogram")
        self.assertEqual(by_name["jaeger_storage_spans_written"]["type"], "counter")

    def test_parses_help_strings(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        by_name = {m["name"]: m for m in metrics}
        self.assertIn("Duration of HTTP server requests", by_name["http_server_duration_milliseconds"]["help"])

    def test_collects_label_keys(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        by_name = {m["name"]: m for m in metrics}
        http_labels = by_name["http_server_duration_milliseconds"]["labels"]
        self.assertIn("http_method", http_labels)
        self.assertIn("http_route", http_labels)
        self.assertIn("le", http_labels)
        storage_labels = by_name["jaeger_storage_spans_written"]["labels"]
        self.assertIn("storage", storage_labels)

    def test_labels_are_sorted(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        for m in metrics:
            self.assertEqual(m["labels"], sorted(m["labels"]))

    def test_metrics_are_sorted_by_name(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        names = [m["name"] for m in metrics]
        self.assertEqual(names, sorted(names))

    def test_excludes_instance_labels(self):
        metrics = parse_snapshot(_SNAPSHOT_WITH_EXCLUDED_LABELS)
        by_name = {m["name"]: m for m in metrics}
        labels = by_name["my_metric"]["labels"]
        self.assertNotIn("service_instance_id", labels)
        self.assertNotIn("otel_scope_version", labels)
        self.assertIn("job", labels)

    def test_metrics_without_labels(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        by_name = {m["name"]: m for m in metrics}
        self.assertEqual(by_name["process_cpu_seconds"]["labels"], [])

    def test_empty_input(self):
        metrics = parse_snapshot("")
        self.assertEqual(metrics, [])

    def test_deduplicates_by_metric_name(self):
        metrics = parse_snapshot(_SNAPSHOT_MEMORY)
        names = [m["name"] for m in metrics]
        self.assertEqual(len(names), len(set(names)))


class TestCollectSnapshots(unittest.TestCase):
    """Tests for collect_snapshots()."""

    def test_reads_snapshot_files(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            path = os.path.join(tmpdir, "metrics_snapshot_memory.txt")
            with open(path, "w") as f:
                f.write(_SNAPSHOT_MEMORY)
            snapshots = collect_snapshots(tmpdir)
            self.assertIn("memory", snapshots)
            self.assertTrue(len(snapshots["memory"]) > 0)

    def test_extracts_backend_name(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            for name in ["metrics_snapshot_memory.txt", "metrics_snapshot_cassandra.txt"]:
                path = os.path.join(tmpdir, name)
                with open(path, "w") as f:
                    f.write(_SNAPSHOT_MEMORY)
            snapshots = collect_snapshots(tmpdir)
            self.assertIn("memory", snapshots)
            self.assertIn("cassandra", snapshots)

    def test_ignores_non_snapshot_files(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            for name in ["other_file.txt", "baseline_metrics.txt", "diff_metrics.txt"]:
                path = os.path.join(tmpdir, name)
                with open(path, "w") as f:
                    f.write("not a snapshot")
            snapshots = collect_snapshots(tmpdir)
            self.assertEqual(len(snapshots), 0)

    def test_reads_from_artifact_subdirectories(self):
        """Supports the gh run download layout: subdir/metrics_snapshot_X.txt."""
        with tempfile.TemporaryDirectory() as tmpdir:
            subdir = os.path.join(tmpdir, "metrics_snapshot_memory")
            os.makedirs(subdir)
            path = os.path.join(subdir, "metrics_snapshot_memory.txt")
            with open(path, "w") as f:
                f.write(_SNAPSHOT_MEMORY)
            snapshots = collect_snapshots(tmpdir)
            self.assertIn("memory", snapshots)
            self.assertTrue(len(snapshots["memory"]) > 0)

    def test_mixed_flat_and_subdirectory_layout(self):
        """Collects from both flat files and subdirectories."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Flat file
            with open(os.path.join(tmpdir, "metrics_snapshot_memory.txt"), "w") as f:
                f.write(_SNAPSHOT_MEMORY)
            # Subdirectory
            subdir = os.path.join(tmpdir, "metrics_snapshot_cassandra")
            os.makedirs(subdir)
            with open(os.path.join(subdir, "metrics_snapshot_cassandra.txt"), "w") as f:
                f.write(_SNAPSHOT_CASSANDRA)
            snapshots = collect_snapshots(tmpdir)
            self.assertIn("memory", snapshots)
            self.assertIn("cassandra", snapshots)

    def test_missing_directory_returns_empty(self):
        snapshots = collect_snapshots("/nonexistent/path")
        self.assertEqual(len(snapshots), 0)


class TestMergeSnapshots(unittest.TestCase):
    """Tests for merge_snapshots()."""

    def test_merges_metrics_from_multiple_backends(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "metrics_snapshot_memory.txt"), "w") as f:
                f.write(_SNAPSHOT_MEMORY)
            with open(os.path.join(tmpdir, "metrics_snapshot_cassandra.txt"), "w") as f:
                f.write(_SNAPSHOT_CASSANDRA)
            snapshots = collect_snapshots(tmpdir)
            merged = merge_snapshots(snapshots)

            names = [m["name"] for m in merged]
            # Cassandra-specific metric should appear (family.name strips _total)
            self.assertIn("jaeger_storage_cassandra_errors", names)
            # Common metric should appear once
            self.assertEqual(names.count("http_server_duration_milliseconds"), 1)

    def test_sources_field_tracks_backends(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
            "cassandra": parse_snapshot(_SNAPSHOT_CASSANDRA),
        }
        merged = merge_snapshots(snapshots)
        by_name = {m["name"]: m for m in merged}

        # Common metric should list both backends
        http_sources = by_name["http_server_duration_milliseconds"]["sources"]
        self.assertIn("memory", http_sources)
        self.assertIn("cassandra", http_sources)

        # Cassandra-only metric (family.name strips _total suffix)
        cass_sources = by_name["jaeger_storage_cassandra_errors"]["sources"]
        self.assertEqual(cass_sources, ["cassandra"])

    def test_labels_are_unioned(self):
        # Both snapshots have http_server_duration_milliseconds with same labels.
        # The union should be the same as either.
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
            "cassandra": parse_snapshot(_SNAPSHOT_CASSANDRA),
        }
        merged = merge_snapshots(snapshots)
        by_name = {m["name"]: m for m in merged}
        labels = by_name["http_server_duration_milliseconds"]["labels"]
        self.assertIn("http_method", labels)
        self.assertIn("http_route", labels)

    def test_merged_is_sorted_by_name(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
            "cassandra": parse_snapshot(_SNAPSHOT_CASSANDRA),
        }
        merged = merge_snapshots(snapshots)
        names = [m["name"] for m in merged]
        self.assertEqual(names, sorted(names))

    def test_empty_snapshots(self):
        merged = merge_snapshots({})
        self.assertEqual(merged, [])


class TestBuildYamlOutput(unittest.TestCase):
    """Tests for build_yaml_output()."""

    def test_output_structure(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
        }
        merged = merge_snapshots(snapshots)
        output = build_yaml_output(merged, snapshots)

        self.assertIn("total_metrics", output)
        self.assertIn("total_backends", output)
        self.assertIn("metrics", output)
        self.assertIn("backends", output)

    def test_total_counts(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
            "cassandra": parse_snapshot(_SNAPSHOT_CASSANDRA),
        }
        merged = merge_snapshots(snapshots)
        output = build_yaml_output(merged, snapshots)

        self.assertEqual(output["total_backends"], 2)
        self.assertGreater(output["total_metrics"], 0)

    def test_backends_contain_metric_lists(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
        }
        merged = merge_snapshots(snapshots)
        output = build_yaml_output(merged, snapshots)

        self.assertIn("memory", output["backends"])
        self.assertIn("count", output["backends"]["memory"])
        self.assertIn("metrics", output["backends"]["memory"])
        self.assertEqual(
            output["backends"]["memory"]["count"],
            len(output["backends"]["memory"]["metrics"]),
        )

    def test_output_is_yaml_serializable(self):
        snapshots = {
            "memory": parse_snapshot(_SNAPSHOT_MEMORY),
            "cassandra": parse_snapshot(_SNAPSHOT_CASSANDRA),
        }
        merged = merge_snapshots(snapshots)
        output = build_yaml_output(merged, snapshots)

        # Should not raise
        yaml_str = yaml.dump(output, default_flow_style=False)
        # Should be parseable back
        reloaded = yaml.safe_load(yaml_str)
        self.assertEqual(reloaded["total_metrics"], output["total_metrics"])
        self.assertEqual(reloaded["total_backends"], output["total_backends"])


class TestEndToEnd(unittest.TestCase):
    """End-to-end test: snapshot files -> YAML output file."""

    def test_full_pipeline(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Write snapshot files
            with open(os.path.join(tmpdir, "metrics_snapshot_memory.txt"), "w") as f:
                f.write(_SNAPSHOT_MEMORY)
            with open(os.path.join(tmpdir, "metrics_snapshot_cassandra.txt"), "w") as f:
                f.write(_SNAPSHOT_CASSANDRA)

            output_path = os.path.join(tmpdir, "metrics.yaml")

            # Run the pipeline
            snapshots = collect_snapshots(tmpdir)
            merged = merge_snapshots(snapshots)
            output = build_yaml_output(merged, snapshots)

            yaml_str = yaml.dump(output, default_flow_style=False, sort_keys=False)
            with open(output_path, "w") as f:
                f.write(yaml_str)

            # Verify file was written
            self.assertTrue(os.path.exists(output_path))

            # Verify content
            with open(output_path, "r") as f:
                loaded = yaml.safe_load(f)
            self.assertEqual(loaded["total_backends"], 2)
            self.assertGreater(loaded["total_metrics"], 0)

            # Verify cassandra-only metric is present (family.name strips _total)
            metric_names = [m["name"] for m in loaded["metrics"]]
            self.assertIn("jaeger_storage_cassandra_errors", metric_names)

            # Verify backends section
            self.assertIn("memory", loaded["backends"])
            self.assertIn("cassandra", loaded["backends"])


if __name__ == "__main__":
    unittest.main()

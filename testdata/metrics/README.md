# This directory holds Prometheus metric name snapshots used by the
# verify-metrics-snapshot composite action to detect backwards-incompatible
# metric renames or removals in CI.
#
# Baseline .txt files are committed here after a first successful run of each
# e2e workflow and must be updated whenever metrics are intentionally added,
# renamed, or removed.

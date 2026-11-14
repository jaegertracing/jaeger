# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

import deploy


class OciScopeTests(unittest.TestCase):
    def execute(self, scope, status="deployed", repo_failure="", status_error=""):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory = root / "examples/oci"
            shutil.copytree(deploy.ROOT / "examples/oci", directory)
            (directory / "helm-charts/.git").mkdir(parents=True)
            (root / "scripts/demo").mkdir(parents=True)
            shutil.copy(deploy.ROOT / "scripts/demo/jaeger-chart.sh", root / "scripts/demo")
            binaries = root / "bin"
            binaries.mkdir()
            fake = binaries / "fake"
            fake.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
command = Path(sys.argv[0]).name
with open(os.environ["TEST_COMMAND_LOG"], "a") as output:
    output.write(json.dumps([command] + sys.argv[1:]) + "\\n")
if command == "helm":
    state = Path(os.environ["TEST_HELM_REPOSITORIES"])
    repositories = json.loads(state.read_text()) if state.exists() else {}
    if sys.argv[1] == "status":
        if os.environ["TEST_STATUS_ERROR"]:
            raise SystemExit("Error: " + os.environ["TEST_STATUS_ERROR"])
        print("STATUS: " + os.environ["TEST_RELEASE_STATUS"])
    elif sys.argv[1:3] == ["repo", "add"]:
        if os.environ["TEST_REPO_FAILURE"] == "add":
            raise SystemExit("repository add failed")
        repositories[sys.argv[3]] = False
        state.write_text(json.dumps(repositories))
    elif sys.argv[1:3] == ["repo", "update"]:
        if os.environ["TEST_REPO_FAILURE"] == "update":
            raise SystemExit("repository update failed")
        for name in sys.argv[3:] or list(repositories):
            if name not in repositories:
                raise SystemExit("Error: repo " + name + " not found")
            repositories[name] = True
        state.write_text(json.dumps(repositories))
    elif sys.argv[1] in ("upgrade", "install") and "prometheus-community/kube-prometheus-stack" in sys.argv:
        if not repositories.get("prometheus-community"):
            raise SystemExit("Prometheus chart repository is missing or its index is unavailable")
''')
            fake.chmod(0o755)
            for name in ("helm", "kubectl", "git"):
                (binaries / name).symlink_to(fake)
            log = root / "commands.jsonl"
            env = dict(os.environ, PATH=str(binaries) + os.pathsep + os.environ["PATH"],
                       JAEGER_DEMO_DEPLOY_SCOPE=scope, TEST_COMMAND_LOG=str(log),
                       TEST_RELEASE_STATUS=status, RUN_PUBLIC_SMOKE_TESTS="false",
                       TEST_HELM_REPOSITORIES=str(root / "repositories.json"), TEST_REPO_FAILURE=repo_failure,
                       TEST_STATUS_ERROR=status_error)
            result = subprocess.run(["bash", str(directory / "deploy-all.sh"), "upgrade"],
                                    env=env, text=True, capture_output=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_jaeger_scope_does_not_touch_monitoring(self):
        result, calls = self.execute("jaeger")
        self.assertEqual(0, result.returncode, result.stderr)
        upgrades = [call for call in calls if call[:2] == ["helm", "upgrade"]]
        self.assertEqual(1, len(upgrades))
        self.assertIn("jaeger", upgrades[0])
        self.assertFalse(any("prometheus" in call or "prometheus-svc.yaml" in call for call in calls))
        checkouts = [call for call in calls if "checkout" in call]
        self.assertEqual(1, len(checkouts))
        self.assertIn("--detach", checkouts[0])

    def test_monitoring_scope_bootstraps_fresh_repository_without_touching_jaeger(self):
        result, calls = self.execute("monitoring")
        self.assertEqual(0, result.returncode, result.stderr)
        upgrades = [call for call in calls if call[:2] == ["helm", "upgrade"]]
        self.assertEqual(1, len(upgrades))
        self.assertIn("prometheus", upgrades[0])
        self.assertFalse(any(call[0] == "git" or "trace-script" in call for call in calls))

    def test_repository_setup_failure_prevents_cluster_mutations(self):
        for operation in ("add", "update"):
            with self.subTest(operation=operation):
                result, calls = self.execute("monitoring", repo_failure=operation)
                self.assertNotEqual(0, result.returncode)
                self.assertIn("repository " + operation + " failed", result.stderr)
                self.assertFalse(any(call[0] == "kubectl" for call in calls))
                self.assertFalse(any(call[:2] == ["helm", "upgrade"] for call in calls))

    def test_failed_monitoring_is_retried_without_uninstalling(self):
        result, calls = self.execute("monitoring", "failed")
        self.assertEqual(0, result.returncode, result.stderr)
        upgrades = [call for call in calls if call[:2] == ["helm", "upgrade"]]
        self.assertEqual(1, len(upgrades))
        self.assertIn("prometheus", upgrades[0])
        self.assertIn("--wait", upgrades[0])
        self.assertEqual("20m0s", upgrades[0][upgrades[0].index("--timeout") + 1])
        self.assertEqual("monitoring-values.yaml", upgrades[0][upgrades[0].index("-f") + 1])
        # Rolling back a failed first install has no earlier revision to return to.
        self.assertNotIn("--atomic", upgrades[0])
        self.assertFalse(any(call[:2] in (["helm", "uninstall"], ["helm", "rollback"]) for call in calls))
        self.assertFalse(any("jaeger" in arg or "trace-script" in arg for call in calls for arg in call))

    def test_pending_or_missing_monitoring_requires_separate_repair(self):
        for status in ("", "pending-install", "pending-upgrade", "pending-rollback", "uninstalling", "unknown",
                       "superseded", "uninstalled"):
            with self.subTest(status=status):
                result, calls = self.execute("monitoring", status)
                self.assertNotEqual(0, result.returncode)
                self.assertIn("repair it separately", result.stderr)
                self.assertEqual([["helm", "status", "prometheus"]], calls)
        with self.subTest(status="release: not found"):
            result, calls = self.execute("monitoring", status_error="release: not found")
            self.assertNotEqual(0, result.returncode)
            self.assertIn("(status: missing); repair it separately", result.stderr)
            self.assertEqual([["helm", "status", "prometheus"]], calls)

    def test_monitoring_status_error_prevents_cluster_mutations(self):
        result, calls = self.execute("monitoring", status_error="Kubernetes cluster unreachable")
        self.assertNotEqual(0, result.returncode)
        self.assertIn("Kubernetes cluster unreachable", result.stderr)
        self.assertNotIn("repair it separately", result.stderr)
        self.assertEqual([["helm", "status", "prometheus"]], calls)

    def test_pinned_chart_renders_grafana_liveness_delay(self):
        script = (deploy.ROOT / "examples/oci/deploy-all.sh").read_text()
        version = re.search(r'^PROMETHEUS_STACK_CHART_VERSION="\$\{PROMETHEUS_STACK_CHART_VERSION:-([^}"]+)\}"$',
                            script, re.MULTILINE)[1]
        # The OCI mirror serves the same chart without the repository's large index.
        result = subprocess.run(
            ["helm", "template", "prometheus", "oci://ghcr.io/prometheus-community/charts/kube-prometheus-stack",
             "--version", version, "--values", str(deploy.ROOT / "examples/oci/monitoring-values.yaml"),
             "--show-only", "charts/grafana/templates/deployment.yaml"],
            text=True, capture_output=True)
        self.assertEqual(0, result.returncode, result.stderr)
        # Only the Grafana container has probes; the chart cannot render a startup probe for it.
        self.assertEqual(1, result.stdout.count("livenessProbe:"))
        self.assertEqual(1, result.stdout.count("readinessProbe:"))
        liveness = result.stdout.split("livenessProbe:")[1].split("readinessProbe:")[0]
        self.assertIn("initialDelaySeconds: 600", liveness)
        self.assertIn("path: /api/health", liveness)

    def test_invalid_scope_has_no_side_effects(self):
        result, calls = self.execute("opensearch")
        self.assertNotEqual(0, result.returncode)
        self.assertEqual([], calls)


if __name__ == "__main__":
    unittest.main()

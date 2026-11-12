# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import deploy


class OciScopeTests(unittest.TestCase):
    def execute(self, scope, status="deployed", repo_failure=""):
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
                       TEST_HELM_REPOSITORIES=str(root / "repositories.json"), TEST_REPO_FAILURE=repo_failure)
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

    def test_failed_monitoring_is_an_error_and_is_not_uninstalled(self):
        result, calls = self.execute("monitoring", "failed")
        self.assertNotEqual(0, result.returncode)
        self.assertIn("repair it separately", result.stderr)
        self.assertEqual([["helm", "status", "prometheus"]], calls)

    def test_invalid_scope_has_no_side_effects(self):
        result, calls = self.execute("opensearch")
        self.assertNotEqual(0, result.returncode)
        self.assertEqual([], calls)


if __name__ == "__main__":
    unittest.main()

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import deploy


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.desired = {name: deploy.fingerprint(name) for name in deploy.COMPONENTS}
        self.env = {"GITHUB_EVENT_NAME": "push", "GITHUB_REF": "refs/heads/main"}

    def test_command_failures_propagate(self):
        result = deploy.run([sys.executable, "-c", "print('ready')"], capture_output=True)
        self.assertEqual("ready\n", result.stdout)
        with self.assertRaises(subprocess.CalledProcessError) as error:
            deploy.run([sys.executable, "-c", "raise SystemExit(3)"])
        self.assertEqual(3, error.exception.returncode)

    def test_unchanged_components_are_skipped_and_failed_revisions_retry(self):
        self.assertEqual([], deploy.selected_components("push", "oci", "jaeger", "upgrade", self.desired, self.desired))
        previous = dict(self.desired, opensearch="old")
        self.assertEqual(["opensearch"], deploy.selected_components("push", "oci", "jaeger", "upgrade", previous, self.desired))

    def test_daily_refreshes_both_jaeger_stacks(self):
        self.assertEqual(["otel-jaeger", "oci-jaeger"], deploy.selected_components(
            "schedule", "oci", "jaeger", "upgrade", self.desired, self.desired))

    def test_manual_routing_and_clean(self):
        for name, (stack, scope, _) in deploy.COMPONENTS.items():
            self.assertEqual([name], deploy.selected_components("workflow_dispatch", stack, scope, "upgrade", {}, self.desired))
        self.assertEqual(list(deploy.COMPONENTS), deploy.selected_components("workflow_dispatch", "all", "all", "clean", {}, self.desired))

    def test_component_changes_do_not_refresh_unrelated_infrastructure(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            shutil.copytree(deploy.ROOT / "examples/otel-demo", root / "examples/otel-demo")
            shutil.copytree(deploy.ROOT / "scripts/demo", root / "scripts/demo")
            script = root / "examples/otel-demo/deploy-all.sh"
            script.write_text(re.sub(r'OPENSEARCH_CHART_VERSION="[^\"]+"', 'OPENSEARCH_CHART_VERSION="99.99.99"', script.read_text()))
            self.assertNotEqual(self.desired["opensearch"], deploy.fingerprint("opensearch", root))
            self.assertEqual(self.desired["otel-jaeger"], deploy.fingerprint("otel-jaeger", root))
            self.assertEqual(self.desired["otel-demo"], deploy.fingerprint("otel-demo", root))
            values = root / "examples/otel-demo/otel-demo-values.yaml"
            values.write_text(values.read_text() + "\n# revised application\n")
            self.assertNotEqual(self.desired["otel-demo"], deploy.fingerprint("otel-demo", root))

    @patch("deploy.run")
    def test_recovery_precedes_upgrade_and_fresh_trace(self, run):
        deploy.deploy_component("opensearch", self.env)
        self.assertEqual("backup-test", run.call_args_list[0].args[0][-1])
        self.assertTrue(run.call_args_list[1].args[0][1].endswith("deploy-all.sh"))
        self.assertTrue(run.call_args_list[2].args[0][1].endswith("verify_trace.py"))
        self.assertEqual("opensearch", run.call_args_list[1].kwargs["env"]["JAEGER_DEMO_DEPLOY_SCOPE"])

    @patch("deploy.run", side_effect=subprocess.CalledProcessError(1, "recovery"))
    def test_recovery_failure_prevents_helm_upgrade(self, run):
        with self.assertRaises(subprocess.CalledProcessError):
            deploy.deploy_component("opensearch", self.env)
        self.assertEqual(1, run.call_count)

    @patch("deploy.run")
    def test_explicit_waiver_skips_only_backup(self, run):
        deploy.deploy_component("opensearch", dict(self.env, JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY="waive"))
        self.assertEqual(2, run.call_count)
        self.assertTrue(run.call_args_list[0].args[0][1].endswith("deploy-all.sh"))
        self.assertTrue(run.call_args_list[1].args[0][1].endswith("verify_trace.py"))

    @patch("deploy.resolve_snapshots")
    @patch("deploy.deploy_component")
    @patch("deploy.run")
    def test_disposable_data_waiver_is_valid_for_automatic_and_full_manual_upgrades(self, run, component, resolve):
        run.return_value.stdout = json.dumps({"data": dict(self.desired, opensearch="old")})
        # Automatic events carry the workflow's input fallbacks, not a full-stack selection.
        for event, ref, stack, scope in (("push", "refs/heads/main", "oci", "jaeger"),
                                         ("schedule", "refs/heads/main", "oci", "jaeger"),
                                         ("workflow_dispatch", "refs/heads/repair", "all", "all")):
            with self.subTest(event=event):
                env = dict(self.env, GITHUB_EVENT_NAME=event, GITHUB_REF=ref, JAEGER_DEMO_STACK=stack,
                           JAEGER_DEMO_DEPLOY_MODE="upgrade", JAEGER_OTEL_DEMO_DEPLOY_SCOPE=scope,
                           JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY="waive")
                self.assertEqual(0, deploy.reconcile(env))
                component.assert_any_call("opensearch", env)
                component.reset_mock()

    def test_hosted_workflow_defaults_to_the_disposable_data_waiver(self):
        workflow = (deploy.ROOT / ".github/workflows/ci-deploy-demo.yml").read_text()
        block = re.search(r"^      opensearch_recovery:\n((?:        .*\n)+)", workflow, re.MULTILINE)
        self.assertIsNotNone(block, "opensearch_recovery input not found")
        self.assertEqual(["waive"], re.findall(r"^        default: (\S+)$", block[1], re.MULTILINE))
        # Every assignment is listed so that a step-level override cannot hide behind the job default.
        self.assertEqual(["${{ github.event.inputs.opensearch_recovery || 'waive' }}"],
                         re.findall(r"^ +JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY: (.+)$", workflow, re.MULTILINE))

    @patch("deploy.run")
    def test_snapshot_exports_are_loaded_and_push_can_use_published_fallback(self, run):
        def export(*args, **kwargs):
            Path(kwargs["env"]["GITHUB_ENV"]).write_text("JAEGER_DEMO_JAEGER_IMAGE_TAG=abc\nIGNORED=x\n")
        run.side_effect = export
        deploy.resolve_snapshots(self.env)
        self.assertEqual("abc", self.env["JAEGER_DEMO_JAEGER_IMAGE_TAG"])
        self.assertNotIn("IGNORED", self.env)
        self.assertEqual("schedule", run.call_args.kwargs["env"]["GITHUB_EVENT_NAME"])

    @patch("deploy.resolve_snapshots")
    @patch("deploy.deploy_component")
    @patch("deploy.run")
    def test_failed_component_is_not_recorded_and_independent_components_continue(self, run, component, resolve):
        run.return_value.stdout = ""
        component.side_effect = [subprocess.CalledProcessError(1, "recovery"), None, None]
        self.assertEqual(1, deploy.reconcile(self.env))
        self.assertEqual(["opensearch", "oci-jaeger", "oci-monitoring"], [call.args[0] for call in component.call_args_list])
        states = [json.loads(call.kwargs["input"])["data"] for call in run.call_args_list if "input" in call.kwargs]
        self.assertNotIn("opensearch", states[-1])
        self.assertEqual({"oci-jaeger", "oci-monitoring"}, set(states[-1]))

    @patch("deploy.resolve_snapshots")
    @patch("deploy.deploy_component")
    @patch("deploy.run")
    def test_only_successful_component_revisions_are_written(self, run, component, resolve):
        previous = dict(self.desired, **{"otel-demo": "old"})
        run.return_value.stdout = json.dumps({"data": previous})
        with tempfile.NamedTemporaryFile() as summary:
            self.env["GITHUB_STEP_SUMMARY"] = summary.name
            self.assertEqual(0, deploy.reconcile(self.env))
            self.assertIn("otel-demo: deployed", Path(summary.name).read_text())
        component.assert_called_once_with("otel-demo", self.env)
        resolve.assert_not_called()
        self.assertEqual(self.desired, json.loads(run.call_args.kwargs["input"])["data"])

    @patch("deploy.resolve_snapshots")
    @patch("deploy.run")
    def test_clean_is_dispatched_once_per_stack(self, run, resolve):
        for policy in ("required", "waive"):
            with self.subTest(policy=policy):
                env = dict(self.env, GITHUB_EVENT_NAME="workflow_dispatch", JAEGER_DEMO_STACK="all",
                           JAEGER_DEMO_DEPLOY_MODE="clean", JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY=policy)
                self.assertEqual(0, deploy.reconcile(env))
                self.assertEqual(2, run.call_count)
                self.assertTrue(all(call.args[0][-1] == "clean" for call in run.call_args_list))
                run.reset_mock()

    @patch("deploy.deploy_component")
    @patch("deploy.run")
    def test_rejects_invalid_or_automatic_unsafe_routes(self, run, component):
        cases = [{"GITHUB_REF": "refs/heads/other"}, {"JAEGER_DEMO_STACK": "invalid"},
                 {"JAEGER_DEMO_DEPLOY_MODE": "invalid"}, {"JAEGER_DEMO_DEPLOY_MODE": "clean"},
                 {"GITHUB_EVENT_NAME": "pull_request"}, {"JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY": "invalid"}]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(ValueError):
                deploy.reconcile(dict(self.env, **case))
        # Rejection must precede any cluster access.
        run.assert_not_called()
        component.assert_not_called()

    @patch("deploy.run")
    def test_manual_invalid_scope_fails(self, run):
        run.return_value.stdout = json.dumps({"data": self.desired})
        with self.assertRaises(ValueError):
            deploy.reconcile(dict(self.env, GITHUB_EVENT_NAME="workflow_dispatch", JAEGER_OTEL_DEMO_DEPLOY_SCOPE="invalid"))


if __name__ == "__main__":
    unittest.main()

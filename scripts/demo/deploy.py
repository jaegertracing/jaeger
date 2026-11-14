#!/usr/bin/env python3

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Reconcile independently deployed demo components under the workflow's lock."""

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[2]
STATE = "jaeger-demo-deployment-revisions"
COMPONENTS = {
    "opensearch": ("otel", "opensearch", [
        "opensearch-values.yaml", "opensearch-dashboard-values.yaml",
        "opensearch-recovery.sh", "opensearch-volume-snapshot-class.yaml",
    ]),
    "otel-jaeger": ("otel", "jaeger", [
        "jaeger-values.yaml", "jaeger-config.yaml", "jaeger-query-service.yaml",
        "load-generator.yaml", "load-generator/requirements.txt", "generate_traces.py",
        "ingress/ingress-jaeger.yaml", "ingress/ingress-opensearch.yaml",
        "ingress/ingress-otel-demo.yaml",
    ]),
    "otel-demo": ("otel", "otel-demo", ["otel-demo-values.yaml"]),
    "oci-jaeger": ("oci", "jaeger", [
        "jaeger-values.yaml", "config.yaml", "ui-config.json", "ingress.yaml",
        "load-generator/load-generator.yaml", "load-generator/generate_traces.py",
        "load-generator/requirements.txt",
    ]),
    "oci-monitoring": ("oci", "monitoring", ["monitoring-values.yaml", "prometheus-svc.yaml"]),
}
CHART_PINS = {
    "opensearch": ("OPENSEARCH_CHART_VERSION", "OPENSEARCH_DASHBOARDS_CHART_VERSION"),
    "otel-demo": ("OTEL_DEMO_CHART_VERSION",),
    "oci-monitoring": ("PROMETHEUS_STACK_CHART_VERSION",),
    "otel-jaeger": ("HOTROD_IMAGE_TAG",),
}
PIN_LINE = re.compile(r'^([A-Z][A-Z0-9_]*_CHART_VERSION|HOTROD_IMAGE_TAG)="([^"\n]+)"$', re.MULTILINE)


def run(args, **kwargs):
    return subprocess.run(args, check=True, text=True, **kwargs)


def fingerprint(component, root=ROOT):
    stack, scope, files = COMPONENTS[component]
    directory = root / "examples" / ("otel-demo" if stack == "otel" else "oci")
    script = (directory / "deploy-all.sh").read_text()
    pins = dict(PIN_LINE.findall(script))
    inputs = [PIN_LINE.sub(lambda m: m[1] + '="<component pin>"', script)]
    inputs += [name + "=" + pins[name] for name in CHART_PINS.get(component, ())]
    inputs += [name + "\n" + (directory / name).read_text() for name in files]
    if scope == "jaeger":
        inputs.append((root / "scripts/demo/jaeger-chart.sh").read_text())
    return hashlib.sha256("\0".join(inputs).encode()).hexdigest()


def selected_components(event, stack, scope, mode, previous, desired):
    if event == "workflow_dispatch":
        return [name for name, (target, part, _) in COMPONENTS.items()
                if stack in (target, "all") and (mode == "clean" or scope == "all" or scope == part)]
    return [name for name in COMPONENTS if previous.get(name) != desired[name]
            or (event == "schedule" and COMPONENTS[name][1] == "jaeger")]


def resolve_snapshots(env):
    # The resolver exports through GITHUB_ENV, which a child cannot apply to us.
    with tempfile.NamedTemporaryFile(mode="r+") as output:
        resolver_env = dict(env, GITHUB_ENV=output.name)
        if env.get("GITHUB_EVENT_NAME") == "push":
            resolver_env["GITHUB_EVENT_NAME"] = "schedule"
        run(["bash", str(ROOT / "scripts/utils/resolve-demo-snapshot-tags.sh")], env=resolver_env)
        for line in output:
            key, value = line.strip().split("=", 1)
            if key in ("JAEGER_DEMO_JAEGER_IMAGE_TAG", "JAEGER_DEMO_HOTROD_IMAGE_TAG", "JAEGER_DEMO_IMAGE_TAG"):
                env[key] = value


def deploy_component(component, env):
    stack, scope, _ = COMPONENTS[component]
    directory = ROOT / "examples" / ("otel-demo" if stack == "otel" else "oci")
    child_env = dict(env, JAEGER_DEMO_STACK=stack, JAEGER_DEMO_DEPLOY_SCOPE=scope,
                     JAEGER_OTEL_DEMO_DEPLOY_SCOPE=scope)
    mode = env.get("JAEGER_DEMO_DEPLOY_MODE", "upgrade")
    if scope == "opensearch" and mode == "upgrade" and env.get("JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY", "required") == "required":
        run(["bash", str(directory / "opensearch-recovery.sh"), "backup-test"], env=child_env)
    run(["bash", str(directory / "deploy-all.sh"), mode], env=child_env)
    if scope == "opensearch":
        version = dict(PIN_LINE.findall((directory / "deploy-all.sh").read_text()))["OPENSEARCH_CHART_VERSION"]
        run(["python3", str(ROOT / "scripts/demo/verify_trace.py"), version], env=child_env)


def reconcile(env, root=ROOT):
    event = env.get("GITHUB_EVENT_NAME", "workflow_dispatch")
    if event not in ("push", "schedule", "workflow_dispatch"):
        raise ValueError("Unsupported demo deployment event")
    if event != "workflow_dispatch" and env.get("GITHUB_REF") != "refs/heads/main":
        raise ValueError("Automatic demo deployment requires main")
    mode = env.get("JAEGER_DEMO_DEPLOY_MODE", "upgrade")
    stack = env.get("JAEGER_DEMO_STACK", "oci")
    scope = env.get("JAEGER_OTEL_DEMO_DEPLOY_SCOPE", "jaeger")
    if stack not in ("oci", "otel", "all") or mode not in ("upgrade", "clean"):
        raise ValueError("Invalid demo stack or mode")
    if event != "workflow_dispatch" and mode != "upgrade":
        raise ValueError("Automatic demo deployments must use upgrade mode")
    if env.get("JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY", "required") not in ("required", "waive"):
        raise ValueError("Invalid OpenSearch recovery policy")
    if event == "workflow_dispatch" and mode == "clean":
        # Clean is an explicit whole-stack operation, not a per-component loop.
        resolve_snapshots(env)
        for target in ("oci", "otel"):
            if stack in (target, "all"):
                child = dict(env, JAEGER_DEMO_STACK=target, JAEGER_DEMO_DEPLOY_SCOPE="all")
                directory = root / "examples" / ("otel-demo" if target == "otel" else "oci")
                run(["bash", str(directory / "deploy-all.sh"), "clean"], env=child)
        return 0

    result = run(["kubectl", "-n", "default", "get", "configmap", STATE,
                  "--ignore-not-found", "-o", "json"], capture_output=True)
    previous = json.loads(result.stdout).get("data", {}) if result.stdout.strip() else {}
    desired = {name: fingerprint(name, root) for name in COMPONENTS}
    selected = selected_components(event, stack, scope, mode, previous, desired)
    if event == "workflow_dispatch" and not selected:
        raise ValueError("Invalid scope for selected demo stack")
    failures = []
    snapshots_resolved = False
    for name in selected:
        if name.startswith("otel-") and "opensearch" in failures:
            print(f"::error::{name}: blocked by failed OpenSearch deployment", flush=True)
            failures.append(name)
            continue
        try:
            if COMPONENTS[name][1] == "jaeger" and not snapshots_resolved:
                resolve_snapshots(env)
                snapshots_resolved = True
            print(f"::group::Deploy {name}", flush=True)
            deploy_component(name, env)
            updated = dict(previous, **{name: desired[name]})
            state = {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {
                "name": STATE, "namespace": "default"}, "data": updated}
            run(["kubectl", "apply", "-f", "-"], input=json.dumps(state))
            previous = updated
            print(f"{name}: deployed successfully", flush=True)
        except subprocess.CalledProcessError as error:
            print(f"::error::{name}: deployment failed ({error.returncode})", flush=True)
            failures.append(name)
        finally:
            print("::endgroup::", flush=True)
    summary = "\n".join(f"- {name}: {'FAILED' if name in failures else 'deployed'}" for name in selected)
    if env.get("GITHUB_STEP_SUMMARY"):
        with open(env["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as output:
            output.write("## Demo deployments\n\n" + (summary or "No component changes.") + "\n")
    return int(bool(failures))


if __name__ == "__main__":
    raise SystemExit(reconcile(dict(os.environ)))

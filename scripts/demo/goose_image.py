# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Publish the demo's pinned Goose image only when its version is absent."""

import hashlib
from pathlib import Path
import re
import subprocess
from urllib.error import HTTPError, URLError
from urllib.request import urlopen


ROOT = Path(__file__).resolve().parents[2]
DOCKERFILE = "scripts/ai-sidecar/goose/Dockerfile"


def image_reference(env, root=ROOT):
    repository = env.get("JAEGER_DEMO_GOOSE_IMAGE_REPOSITORY", "jaegertracing/jaeger-goose-snapshot")
    if not re.fullmatch(r"[a-z0-9_-]+/[a-z0-9_-]+", repository):
        raise ValueError("Goose image repository must be a Docker Hub namespace/name")
    source = (root / DOCKERFILE).read_bytes()
    version = re.search(rb"^ARG GOOSE_VERSION=(v[0-9]+\.[0-9]+\.[0-9]+)$", source, re.MULTILINE)
    if not version:
        raise ValueError("Goose Dockerfile must pin a release version")
    tag = version[1].decode() + "-" + hashlib.sha256(source).hexdigest()[:12]
    return f"{repository}:{tag}"


def ensure_image(env, root=ROOT):
    # Manual dispatches can select branches, but registry credentials are only
    # for trusted main. Keep this guard next to the publishing operation too.
    if env.get("GITHUB_REF") != "refs/heads/main":
        raise ValueError("Goose image publishing requires main")
    image = image_reference(env, root)
    repository, tag = image.split(":")
    try:
        with urlopen(f"https://hub.docker.com/v2/repositories/{repository}/tags/{tag}", timeout=30):
            print(f"Using published Goose image {image}", flush=True)
            return image
    except HTTPError as error:
        if error.code != 404:
            raise RuntimeError(f"Goose image lookup failed (HTTP {error.code})") from error
    except (URLError, TimeoutError) as error:
        raise RuntimeError("Goose image lookup failed; refusing to publish on an unknown cache state") from error

    subprocess.run(["bash", str(root / "scripts/utils/docker-login.sh")], check=True, env=env)
    subprocess.run(["docker", "buildx", "build", "--platform", "linux/amd64,linux/arm64",
                    "--tag", image, "--tag", f"quay.io/{image}", "--push",
                    str(root / "scripts/ai-sidecar/goose")], check=True, env=env)
    return image

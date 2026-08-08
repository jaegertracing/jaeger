# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Plumbing shared by every ACP sidecar in this tree.

Each sidecar is its own uv project with its own model-vendor dependencies, but
they all present the same ACP surface to the Jaeger AI gateway. Keeping the
WebSocket/ACP bridge here makes that sameness structural rather than a thing two
copies of the same file happen to agree on.

Sidecars reach this package by putting the parent directory on ``sys.path``
before importing it; see each sidecar's ``main.py`` and ``conftest.py``.
"""

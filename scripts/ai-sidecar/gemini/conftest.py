# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Put scripts/ai-sidecar on sys.path so tests can import the shared package.

``main.py`` does the same thing for normal runs. pytest imports the sidecar
modules directly without going through ``main``, so it needs its own hook.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

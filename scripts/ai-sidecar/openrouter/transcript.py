# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Append-only JSONL record of what a turn actually did.

Set EVAL_TRANSCRIPT to a file path and every loop event lands there as one JSON
object per line. Leave it unset and every hook is a no-op — this module exists
to make a turn auditable after the fact, and must have zero effect on the
model path.

What lands here is the raw material for a cross-model comparison: the exact
tool_calls block a model emitted, in the order it emitted it. It is deliberately
verbatim. Summarizing it here would throw away the thing being measured.
"""

import json
import logging
import os
import time
from typing import Any

logger = logging.getLogger(__name__)

ENV_VAR = "EVAL_TRANSCRIPT"


class Transcript:
    """Writes turn events as JSONL, or does nothing when unconfigured."""

    def __init__(self, path: str | None):
        self._path = path or None

    @classmethod
    def from_env(cls) -> "Transcript":
        return cls(os.environ.get(ENV_VAR, "").strip() or None)

    @property
    def enabled(self) -> bool:
        return self._path is not None

    def emit(self, session_id: str, event: str, **payload: Any) -> None:
        if self._path is None:
            return
        record = {
            "ts": time.time(),
            "session_id": session_id,
            "event": event,
            **payload,
        }
        try:
            with open(self._path, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(record, default=str) + "\n")
        except OSError as exc:
            # Observability must never take the turn down with it.
            logger.warning("Transcript write to %s failed: %s", self._path, exc)

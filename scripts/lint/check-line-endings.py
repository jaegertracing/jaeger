#!/usr/bin/env python3
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# check-line-endings.py — lint and optionally fix line-ending issues in all
# Git-tracked text files.  The following violations are detected and corrected:
#
#   * Trailing whitespace (spaces or tabs immediately before the line ending)
#   * Missing newline at end of file
#   * Windows-style CRLF line endings (\r\n); this project uses Unix LF (\n) only
#
# Usage:
#   check-line-endings.py        # lint mode: report violations, exit 1 if any found
#   check-line-endings.py -u     # fix  mode: rewrite offending files in-place
#
# Files are processed in parallel using a thread pool; each file is read once
# in Python with no subprocesses spawned per file.

import argparse
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path


def get_tracked_files() -> list[bytes]:
    try:
        result = subprocess.run(
            ["git", "ls-files", "-z"], capture_output=True, check=True
        )
    except FileNotFoundError:
        raise SystemExit("error: 'git' not found on PATH")
    except subprocess.CalledProcessError as exc:
        stderr = exc.stderr.decode(errors="replace").strip()
        msg = "error: 'git ls-files -z' failed"
        raise SystemExit(f"{msg}: {stderr}" if stderr else msg)
    return [f for f in result.stdout.split(b"\0") if f]


def process_file(path_bytes: bytes, fix: bool) -> list[str]:
    path = Path(path_bytes.decode())
    if not path.is_file():
        return []

    try:
        content = path.read_bytes()
    except OSError:
        return []

    # Skip empty files — they have no content to check.
    if not content:
        return []

    # Skip binary files — any null byte is a reliable indicator.
    if b"\x00" in content:
        return []

    # Check for each violation.  Strip \r before checking trailing whitespace
    # so that CRLF files don't produce false "trailing whitespace" reports.
    lines = [line.rstrip(b"\r") for line in content.split(b"\n")]
    has_crlf = b"\r" in content
    has_trailing_space = any(line != line.rstrip(b" \t") for line in lines)
    has_newline = content[-1:] == b"\n"

    if not has_crlf and not has_trailing_space and has_newline:
        return []

    if fix:
        # Normalise in one pass: CRLF→LF, bare CR→nothing, then strip
        # trailing whitespace from each line, then ensure terminal newline.
        fixed = content.replace(b"\r\n", b"\n").replace(b"\r", b"")
        fixed = b"\n".join(line.rstrip(b" \t") for line in fixed.split(b"\n"))
        if not fixed.endswith(b"\n"):
            fixed += b"\n"
        if fixed != content:
            path.write_bytes(fixed)
        return [f"Fixing: {path}"]

    msgs = [f"Lint error: {path}"]
    if has_trailing_space:
        msgs.append("  -> Trailing whitespace found")
    if has_crlf:
        msgs.append("  -> Windows-style CRLF line endings found (expected Unix LF)")
    if not has_newline:
        msgs.append("  -> Missing newline at end of file")
    return msgs


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Lint / fix line endings in all Git-tracked text files."
    )
    parser.add_argument(
        "-u", action="store_true", help="fix mode: rewrite offending files in-place"
    )
    args = parser.parse_args()

    files = get_tracked_files()
    results: list[tuple[bytes, list[str]]] = []

    with ThreadPoolExecutor() as executor:
        futures = {executor.submit(process_file, f, args.u): f for f in files}
        for future in as_completed(futures):
            msgs = future.result()
            if msgs:
                results.append((futures[future], msgs))

    # Sort by filename for deterministic output.
    results.sort(key=lambda x: x[0])
    errors = False
    for _, msgs in results:
        for line in msgs:
            print(line)
        if not args.u:
            errors = True

    sys.exit(1 if errors else 0)


if __name__ == "__main__":
    main()

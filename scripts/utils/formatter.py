#!/usr/bin/env python3
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import re
import sys

def extract_section_from_file(file_path, start_marker, end_marker):
    try:
        with open(file_path, 'r') as f:
            text = f.read()
    except Exception as e:
        raise Exception(f"error reading file: {e}") from e

    start_index = text.find(start_marker)
    if start_index == -1:
        raise Exception(f"start marker {start_marker!r} not found")
    start_index += len(start_marker)

    end_index = text.find(end_marker)
    if end_index == -1:
        raise Exception(f"end marker {end_marker!r} not found")

    return text[start_index:end_index]

def extract_after_start(file_path, start_marker):
    try:
        with open(file_path, 'r') as f:
            text = f.read()
    except Exception as e:
        raise Exception(f"error reading file: {e}") from e

    start_index = text.find(start_marker)
    if start_index == -1:
        raise Exception(f"start marker {start_marker!r} not found")
    start_index += len(start_marker)

    # Mimic Go's "len(text)-1" slicing (discarding the final character)
    end_index = len(text) - 1
    return text[start_index:end_index]

def main():
    
    loc = sys.argv[1]
    try:
        backend_file_name = "RELEASE.md"
        backend_start_marker = "<!-- BEGIN_BACKEND -->"
        backend_end_marker = "<!-- END_BACKEND -->"

        backend_section = extract_section_from_file(backend_file_name, backend_start_marker, backend_end_marker)
    except Exception as e:
        sys.exit(f"Failed to extract backendSection: {e}")

    manual_start = "<!-- BEGIN_MANUAL -->"
    manual_end = "<!-- END_MANUAL -->"
    manual_pattern = f"{manual_start}.*?{manual_end}"
    backend_section = re.sub(manual_pattern, '', backend_section, flags=re.DOTALL)
    re_star = re.compile(r'(\n\s*)(\*)(\s)')
    backend_section = re_star.sub(r'\1\2 [ ]\3', backend_section)
    re_num = re.compile(r'(\n\s*)([0-9]*\.)(\s)')
    backend_section = re_num.sub(r'\1* [ ]\3', backend_section)

    try:
        doc_filename = loc
        doc_start_marker = "# Release instructions"
        doc_end_marker = "### Auto-generated documentation for CLI flags"
        doc_section = extract_section_from_file(doc_filename, doc_start_marker, doc_end_marker)
    except Exception as e:
        sys.exit(f"Failed to extract documentation section: {e}")

    doc_section = re.sub(manual_pattern, '', doc_section, flags=re.DOTALL)
    re_dash = re.compile(r'(\n\s*)(\-)')
    doc_section = re_dash.sub(r'\1* [ ]', doc_section)

    try:
        ui_filename = "jaeger-ui/RELEASE.md"
        ui_start_marker = "# Cutting a Jaeger UI release"
        ui_section = extract_after_start(ui_filename, ui_start_marker)
    except Exception as e:
        sys.exit(f"Failed to extract UI section: {e}")

    ui_section = re.sub(manual_pattern, '', ui_section, flags=re.DOTALL)
    ui_section = re_dash.sub(r'\1* [ ]', ui_section)
    ui_section = re_num.sub(r'\1* [ ]\3', ui_section)

    print("# UI Release")
    print(ui_section)
    print("# Backend Release")
    print(backend_section)
    print("# Doc Release")
    print(doc_section)

if __name__ == "__main__":
    main()

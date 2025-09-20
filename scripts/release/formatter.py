#!/usr/bin/env python3
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import re
import sys

def extract_section_from_file(file_path, start_marker, end_marker):
    with open(file_path, 'r') as f:
        text = f.read()
    start_index = text.find(start_marker)
    if start_index == -1:
        raise Exception(f"start marker {start_marker!r} not found")
    start_index += len(start_marker)
    end_index = text.find(end_marker)
    if end_index == -1:
        raise Exception(f"end marker {end_marker!r} not found")
    return text[start_index:end_index]

def replace_star(text):
    re_star = re.compile(r'(\n\s*)(\*)(\s)')
    text = re_star.sub(r'\1\2 [ ]\3', text)
    return text

def replace_dash(text):
    re_dash = re.compile(r'(\n\s*)(\-)')
    text = re_dash.sub(r'\1* [ ]', text)
    return text

def replace_num(text):
    re_num = re.compile(r'(\n\s*)([0-9]*\.)(\s)')
    text = re_num.sub(r'\1* [ ]\3', text)
    return text

def replace_version(ui_text, backend_text, doc_text, pattern, ver):
    ui_text = re.sub(pattern, ver, ui_text)
    backend_text = re.sub(pattern, ver, backend_text)
    doc_text = re.sub(pattern, ver, doc_text)
    return ui_text, backend_text, doc_text

def main():
    loc = sys.argv[1]
    v1 = sys.argv[2].strip() if len(sys.argv) > 2 else ""
    v2 = sys.argv[3].strip() if len(sys.argv) > 3 else ""

    try:
        backend_section = extract_section_from_file("RELEASE.md", "<!-- BEGIN_CHECKLIST -->", "<!-- END_CHECKLIST -->")
    except Exception as e:
        sys.exit(f"Failed to extract backend section: {e}")

    backend_section = replace_star(backend_section)
    backend_section = replace_num(backend_section)

    try:
        doc_section = extract_section_from_file(loc, "<!-- BEGIN_CHECKLIST -->", "<!-- END_CHECKLIST -->")
    except Exception as e:
        sys.exit(f"Failed to extract documentation section: {e}")
    doc_section = replace_dash(doc_section)

    try:
        ui_section = extract_section_from_file("jaeger-ui/RELEASE.md", "<!-- BEGIN_CHECKLIST -->", "<!-- END_CHECKLIST -->")
    except Exception as e:
        sys.exit(f"Failed to extract UI section: {e}")

    ui_section = replace_dash(ui_section)
    ui_section = replace_num(ui_section)

    # Replace v2 versions (primary focus)
    v2_pattern = r'2.x.x'
    ui_section, backend_section, doc_section = replace_version(ui_section, backend_section, doc_section, v2_pattern, v2)

    # TODO: Remove v1 version replacement after final v1 release (early 2026)
    if v1:
        v1_pattern = r'(?:X\.Y\.Z|1\.[0-9]+\.[0-9]+|1\.x\.x)'
        ui_section, backend_section, doc_section = replace_version(ui_section, backend_section, doc_section, v1_pattern, v1)
    else:
        # Add deprecation notice if v1 is skipped
        deprecation_note = "\n**Note:** v1 releases are in maintenance mode. Only 3 more v1 releases planned before full v2 transition.\n"
        ui_section = deprecation_note + ui_section
        backend_section = deprecation_note + backend_section
        doc_section = deprecation_note + doc_section

    print("# UI Release")
    print(ui_section)
    print("# Backend Release")
    print(backend_section)
    print("# Doc Release")
    print(doc_section)

if __name__ == "__main__":
    main()

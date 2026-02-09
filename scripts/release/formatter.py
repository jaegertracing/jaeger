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

def replace_bullets(text):
    """Convert all bullet point types (*, -, numbered) to checkbox format (* [ ]).
    
    Handles:
    - Star bullets: * item -> * [ ] item
    - Dash bullets: - item -> * [ ] item  
    - Numbered items: 1. item -> * [ ] item
    """
    # Star bullets: keep the star, add checkbox
    text = re.sub(r'(\n\s*)(\*)(\s)(?!\[)', r'\1\2 [ ]\3', text)
    # Dash bullets: replace dash with star checkbox
    text = re.sub(r'(\n\s*)(\-)(\s+)(?!\[)', r'\1* [ ]\3', text)
    # Numbered items: replace number with star checkbox
    text = re.sub(r'(\n\s*)([0-9]*\.)(\s)(?!\[)', r'\1* [ ]\3', text)
    return text

def convert_automated_to_bash_blocks(text):
    """Convert inline backtick commands after **Automated**: to fenced bash blocks.
    
    Transforms:
        - **Automated**: `command here`
    Into:
        - **Automated**:
          ```bash
          command here
          ```
    
    This enables GitHub's copy button for the command blocks.
    """
    # Pattern matches: **Automated**: followed by optional whitespace, then `command`
    # Captures the leading whitespace/indent, and the command inside backticks
    pattern = r'(\n\s*-\s*)\*\*Automated\*\*:\s*`([^`]+)`'
    
    def replacement(match):
        indent = match.group(1)
        command = match.group(2)
        # Calculate additional indent for the code block (2 spaces after the list item indent)
        base_indent = indent.rstrip('-').rstrip()
        code_indent = base_indent + '  '
        return f'{indent}**Automated**:\n{code_indent}```bash\n{code_indent}{command}\n{code_indent}```'
    
    return re.sub(pattern, replacement, text)

def replace_version(ui_text, backend_text, doc_text, pattern, ver):
    ui_text = re.sub(pattern, ver, ui_text)
    backend_text = re.sub(pattern, ver, backend_text)
    doc_text = re.sub(pattern, ver, doc_text)
    return ui_text, backend_text, doc_text

def fetch_content(file_name):
    start_marker = "<!-- BEGIN_CHECKLIST -->"
    end_marker = "<!-- END_CHECKLIST -->"
    text = extract_section_from_file(file_name, start_marker, end_marker)
    return text

def main():
    
    version = sys.argv[1]
    ui_filename = sys.argv[2]
    loc = sys.argv[3]

    try:
        backend_file_name = "RELEASE.md"
        backend_section = fetch_content(backend_file_name)
    except Exception as e:
        sys.exit(f"Failed to extract backendSection: {e}")
    backend_section = convert_automated_to_bash_blocks(backend_section)
    backend_section = replace_bullets(backend_section)
    try:
        doc_filename = loc
        doc_section = fetch_content(doc_filename)
    except Exception as e:
        sys.exit(f"Failed to extract documentation section: {e}")
    doc_section = convert_automated_to_bash_blocks(doc_section)
    doc_section = replace_bullets(doc_section)

    try:
        ui_section = fetch_content(ui_filename)
    except Exception as e:
        sys.exit(f"Failed to extract UI section: {e}")
    ui_section = convert_automated_to_bash_blocks(ui_section)
    ui_section = replace_bullets(ui_section)

    # Concrete version - replace version patterns with the single version
    version_pattern = r'(?:X\.Y\.Z|[0-9]+\.[0-9]+\.[0-9]+|[0-9]+\.x\.x)'
    ui_section, backend_section, doc_section = replace_version(ui_section, backend_section, doc_section, version_pattern, version)

    print("# UI Release")
    print(ui_section)
    print("# Backend Release")
    print(backend_section)
    print("# Doc Release")
    print(doc_section)

if __name__ == "__main__":
    main()

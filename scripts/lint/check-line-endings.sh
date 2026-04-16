#!/usr/bin/env bash
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# check-line-endings.sh — lint and optionally fix line-ending issues in all
# Git-tracked text files.  The following violations are detected and corrected:
#
#   * Trailing whitespace (spaces or tabs immediately before the line ending)
#   * Missing newline at end of file
#   * Windows-style CRLF line endings (\r\n); this project uses Unix LF (\n) only
#
# Usage:
#   check-line-endings.sh        # lint mode: report violations, exit 1 if any found
#   check-line-endings.sh -u     # fix  mode: rewrite offending files in-place
#
# macOS note: GNU sed (gsed) is required for in-place editing because BSD sed
# has different -i semantics.  Install with: brew install gnu-sed

# Use GNU sed on macOS (same pattern as the project Makefile).
if [[ "$(uname)" == "Darwin" ]]; then
    SED=gsed
else
    SED=sed
fi

FIX_MODE=false

while getopts "u" opt; do
    case $opt in
        u) FIX_MODE=true ;;
        *) echo "Usage: $0 [-u]"; exit 1 ;;
    esac
done

EXIT_CODE=0

# Use process substitution instead of piping into `while` so that writes to
# EXIT_CODE inside the loop are visible to the outer shell (a pipe creates a
# subshell whose variable mutations are discarded on exit).
while IFS= read -r -d $'\0' FILE; do
    # Skip symlinks and files that no longer exist on disk.
    [ ! -f "$FILE" ] && continue

    HAS_TRAILING_SPACE=$(grep -q '[[:blank:]]$' "$FILE" && echo true || echo false)
    HAS_CRLF=$(grep -q $'\r' "$FILE" && echo true || echo false)
    # tail -c1 | read -r exits 1 when the last byte is not a newline.
    HAS_NEWLINE=$(tail -c1 "$FILE" | read -r _ && echo true || echo false)

    if [ "$HAS_TRAILING_SPACE" = true ] || [ "$HAS_CRLF" = true ] || [ "$HAS_NEWLINE" = false ]; then
        if [ "$FIX_MODE" = true ]; then
            echo "Fixing: $FILE"
            # Strip carriage returns (CRLF -> LF, or bare CR -> nothing).
            "$SED" -i 's/\r//' "$FILE"
            # Remove trailing spaces and tabs.
            "$SED" -i 's/[[:blank:]]*$//' "$FILE"
            # Append a newline if the file does not already end with one.
            "$SED" -i -e '$a\' "$FILE"
        else
            echo "Lint error: $FILE"
            [ "$HAS_TRAILING_SPACE" = true ] && echo "  -> Trailing whitespace found"
            [ "$HAS_CRLF" = true ]           && echo "  -> Windows-style CRLF line endings found (expected Unix LF)"
            [ "$HAS_NEWLINE" = false ]        && echo "  -> Missing newline at end of file"
            EXIT_CODE=1
        fi
    fi
done < <(git ls-files -z)

exit $EXIT_CODE

#!/bin/bash

# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script checks that all directories with go files
# have at least one *_test.go file or a .nocover file.

set -euo pipefail

NO_TEST_FILE_DIRS=""
total_pkgs=0

# shellcheck disable=SC2048
for dir in $*; do
  ((total_pkgs+=1))
  mainFile=$(find "${dir}" -maxdepth 1 -name 'main.go')
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  if [ -z "${testFiles}" ]; then
    if [ -n "${mainFile}" ]; then
      continue # single main does not require tests
    fi
    if [ -e "${dir}/.nocover" ]; then
      reason=$(cat "${dir}/.nocover")
      if [ "${reason}" == "" ]; then
        echo "error: ${dir}/.nocover must specify reason" >&2
        exit 1
      fi
      echo "Package excluded from coverage: ${dir}"
      echo "  reason: ${reason}" | sed "s/FIXME/🔴 FIXME/"
      continue
    fi
    if [ -z "${NO_TEST_FILE_DIRS}" ]; then
      NO_TEST_FILE_DIRS="${dir}"
    else
      NO_TEST_FILE_DIRS="${NO_TEST_FILE_DIRS} ${dir}"
    fi
  fi
done

if [ -n "${NO_TEST_FILE_DIRS}" ]; then
  echo "*** directories without *_test.go files:" >&2
  echo "${NO_TEST_FILE_DIRS}" | tr ' ' '\n' >&2
  echo "error: at least one *_test.go file must be in all directories with go files so that they are counted for code coverage" >&2
  echo "       if no tests are possible for a package (e.g. it only defines types), create empty_test.go" >&2
  exit 1
else
  echo "✅ Info(check-test-files): no issues after scanning ${total_pkgs} package(s)."
fi

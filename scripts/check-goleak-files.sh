#!/bin/bash

set -euo pipefail

NO_GOLEAK_FILE_DIRS=""

# shellcheck disable=SC2048
for dir in $*; do
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  for test in ${testFiles}; do
    if grep -q "TestMain" "${test}" | grep -q "goleak" "${test}"; then
      break
    elif [[ ${test} == "/" ]]; then
      continue
    else
      if [ -z "${NO_GOLEAK_FILE_DIRS}" ]; then
        NO_GOLEAK_FILE_DIRS="${dir}"
      else
        NO_GOLEAK_FILE_DIRS="${NO_GOLEAK_FILE_DIRS} ${dir}"
      fi
    fi
  done
done

if [ -n "${NO_GOLEAK_FILE_DIRS}" ]; then
  echo "*** directories without Goleak implemented:" >&2
  echo "${NO_GOLEAK_FILE_DIRS}" | tr ' ' '\n' | sort -u >&2
  exit 1
fi
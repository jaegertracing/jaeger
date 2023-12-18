#!/bin/bash

set -euo pipefail

NO_GOLEAK_FILE_DIRS=""

# shellcheck disable=SC2048
for dir in $*; do
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  i=0
  for test in ${testFiles}; do
    if grep -q "TestMain" "${test}" | grep -q "goleak.VerifyTestMain" "${test}"; then
      i=$((i + 1))
    elif [[ ${dir} == "./" ]]; then
      i=$((i + 1))
      continue
    # else
    #   NO_GOLEAK_FILE_DIRS="${NO_GOLEAK_FILE_DIRS} ${dir}"
    else
      continue
    fi
  done
  if ((i == 0)); then
    NO_GOLEAK_FILE_DIRS="${NO_GOLEAK_FILE_DIRS} ${dir}"
  fi
done

echo "Directories Without Goleak Implemented:"
echo "${NO_GOLEAK_FILE_DIRS}" | tr ' ' '\n' | sort -u | tee /dev/stderr | echo "Total=""$(wc -l)" 
 
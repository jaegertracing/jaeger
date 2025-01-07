#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

bad_pkgs=0
total_pkgs=0
failed_pkgs=0
invalid_use_pkgs=0

# shellcheck disable=SC2048
for dir in $*; do
  ((total_pkgs+=1))
  if [[ -f "${dir}/.nocover" ]]; then
    continue
  fi
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  if [[ -z "$testFiles" ]]; then
    continue
  fi
  good=0
  invalid=0
  for test in ${testFiles}; do
    if grep -q "TestMain" "${test}" && grep -q "testutils.VerifyGoLeaks" "${test}"; then
      if [ "${dir}" != "./plugin/storage/integration/" ] && [ "${dir}" != "./cmd/jaeger/internal/integration/" ] &&  grep -q "testutils.VerifyGoLeaksForES" "${test}"; then
          invalid=1
          break
      fi
      good=1
      break
    fi
  done

  if ((good == 0)); then
    if ((invalid == 1)); then
      echo "Error(check-goleak): VerifyGoLeaksForES should only be used in integration package but it is used in ${dir} also"
      ((invalid_use_pkgs+=1))
    else
      echo "Error(check-goleak): no goleak check in package ${dir}"
      ((bad_pkgs+=1))
      ((failed_pkgs+=1))
    fi
  fi
done

function help() {
  echo "	See pkg/version/package_test.go as example of adding the checks."
}

if ((failed_pkgs > 0)); then
  echo "⛔ Fatal(check-goleak): no goleak check in ${bad_pkgs} package(s), ${failed_pkgs} of which not allowed."
  help
  exit 1
elif ((invalid_use_pkgs > 0)); then
  echo "⛔ Fatal(check-goleak): use of VerifyGoLeaksForES in package(s) ${invalid_use_pkgs} which is not allowed"
  help
  exit 1
elif ((bad_pkgs > 0)); then
  echo "🐞 Warning(check-goleak): no goleak check in ${bad_pkgs} package(s)."
  help
else
  echo "✅ Info(check-goleak): no issues after scanning ${total_pkgs} package(s)."
fi

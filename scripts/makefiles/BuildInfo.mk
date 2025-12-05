# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

GIT_SHA=$(shell git rev-parse HEAD)
DATE=$(shell TZ=UTC0 git show --quiet --date='format-local:%Y-%m-%dT%H:%M:%SZ' --format="%cd")
# Defer evaluation of semver tags until actually needed, using trick from StackOverflow:
# https://stackoverflow.com/questions/44114466/how-to-declare-a-deferred-variable-that-is-computed-only-once-for-all
GIT_CLOSEST_TAG = $(eval GIT_CLOSEST_TAG := $(shell scripts/utils/compute-version.sh v2))$(GIT_CLOSEST_TAG)

# args: (1) - name, (2) - value
define buildinfo
  $(JAEGER_IMPORT_PATH)/internal/version.$(1)=$(2)
endef
define buildinfoflags
  -ldflags "-X $(call buildinfo,commitSHA,$(GIT_SHA)) -X $(call buildinfo,latestVersion,$(GIT_CLOSEST_TAG)) -X $(call buildinfo,date,$(DATE))"
endef
BUILD_INFO=$(call buildinfoflags)

# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

GIT_SHA=$(shell git rev-parse HEAD)
DATE=$(shell TZ=UTC0 git show --quiet --date='format-local:%Y-%m-%dT%H:%M:%SZ' --format="%cd")
# Defer evaluation of semver tags until actually needed, using trick from StackOverflow:
# https://stackoverflow.com/questions/44114466/how-to-declare-a-deferred-variable-that-is-computed-only-once-for-all
GIT_CLOSEST_TAG_V1 = $(eval GIT_CLOSEST_TAG_V1 := $(shell scripts/utils/compute-version.sh v1))$(GIT_CLOSEST_TAG_V1)
GIT_CLOSEST_TAG_V2 = $(eval GIT_CLOSEST_TAG_V2 := $(shell scripts/utils/compute-version.sh v2))$(GIT_CLOSEST_TAG_V2)

# args: (1) - name, (2) - value
define buildinfo
  $(JAEGER_IMPORT_PATH)/pkg/version.$(1)=$(2)
endef
# args (1) - V1|V2
define buildinfoflags
  -ldflags "-X $(call buildinfo,commitSHA,$(GIT_SHA)) -X $(call buildinfo,latestVersion,$(GIT_CLOSEST_TAG_$(1))) -X $(call buildinfo,date,$(DATE))"
endef
BUILD_INFO=$(call buildinfoflags,V1)
BUILD_INFO_V2=$(call buildinfoflags,V2)

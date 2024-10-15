# Copyright (c) 2024 The Jaeger Authors.
# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

SYSOFILE=resource.syso

# Magic values:
# - LangID "0409" is "US-English".
# - CharsetID "04B0" translates to decimal 1200 for "Unicode".
# - FileOS "040004" defines the Windows kernel "Windows NT".
# - FileType "01" is "Application".
# https://learn.microsoft.com/en-us/windows/win32/menurc/versioninfo-resource
define VERSIONINFO
{
    "FixedFileInfo": {
        "FileVersion": {
            "Major": $(SEMVER_MAJOR),
            "Minor": $(SEMVER_MINOR),
            "Patch": $(SEMVER_PATCH),
            "Build": 0
        },
        "ProductVersion": {
            "Major": $(SEMVER_MAJOR),
            "Minor": $(SEMVER_MINOR),
            "Patch": $(SEMVER_PATCH),
            "Build": 0
        },
        "FileFlagsMask": "3f",
        "FileFlags ": "00",
        "FileOS": "040004",
        "FileType": "01",
        "FileSubType": "00"
    },
    "StringFileInfo": {
        "FileDescription": "$(NAME)",
        "FileVersion": "$(SEMVER_MAJOR).$(SEMVER_MINOR).$(SEMVER_PATCH).0",
        "LegalCopyright": "2015-2024 The Jaeger Project Authors",
		"ProductName": "$(NAME)",
        "ProductVersion": "$(SEMVER_MAJOR).$(SEMVER_MINOR).$(SEMVER_PATCH).0"
    },
    "VarFileInfo": {
        "Translation": {
            "LangID": "0409",
            "CharsetID": "04B0"
        }
    }
}
endef

export VERSIONINFO

.PHONY: _build_syso_once
_build_syso_once:
	echo $$VERSIONINFO
	echo $$VERSIONINFO | $(GOVERSIONINFO) -o="$(PKGPATH)/$(SYSOFILE)" -

define _build_syso_macro
	$(MAKE) _build_syso_once NAME="$(1)" PKGPATH="$(2)" SEMVER_MAJOR=$(SEMVER_MAJOR) SEMVER_MINOR=$(SEMVER_MINOR) SEMVER_PATCH=$(SEMVER_PATCH)
endef

.PHONY: _build-syso
_build-syso: $(GOVERSIONINFO)
	$(eval SEMVER_ALL := $(shell scripts/compute-version.sh -s v1))
	$(eval SEMVER_MAJOR := $(word 2, $(SEMVER_ALL)))
	$(eval SEMVER_MINOR := $(word 3, $(SEMVER_ALL)))
	$(eval SEMVER_PATCH := $(word 4, $(SEMVER_ALL)))
	$(call _build_syso_macro,Jaeger Collector,cmd/collector)
	$(call _build_syso_macro,Jaeger Query,cmd/query)
	$(call _build_syso_macro,Jaeger Ingester,cmd/ingester)
	$(call _build_syso_macro,Jaeger Remote Storage,cmd/remote-storage)
	$(call _build_syso_macro,Jaeger All-In-One,cmd/all-in-one)
	$(call _build_syso_macro,Jaeger Tracegen,cmd/tracegen)
	$(call _build_syso_macro,Jaeger Anonymizer,cmd/anonymizer)
	$(call _build_syso_macro,Jaeger ES-Index-Cleaner,cmd/es-index-cleaner)
	$(call _build_syso_macro,Jaeger ES-Rollover,cmd/es-rollover)
	# TODO in the future this should be in v2
	$(call _build_syso_macro,Jaeger V2,cmd/jaeger)

.PHONY: _clean-syso
_clean-syso:
	rm ./cmd/*/$(SYSOFILE)

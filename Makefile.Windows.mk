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

.PHONY: _prepare-syso-helper
_prepare-syso-helper:
	echo $(NAME)
	echo $$VERSIONINFO
	echo $$VERSIONINFO | $(GOVERSIONINFO) -o="$(PKGPATH)/$(SYSOFILE)" -

.PHONY: _prepare-syso
_prepare-syso: $(GOVERSIONINFO)
	$(eval SEMVER_ALL := $(shell QUIET=1 scripts/compute-version.sh v1))
	$(eval SEMVER_MAJOR := $(word 2, $(SEMVER_ALL)))
	$(eval SEMVER_MINOR := $(word 3, $(SEMVER_ALL)))
	$(eval SEMVER_PATCH := $(word 4, $(SEMVER_ALL)))
	echo SEMVER_MAJOR=$(SEMVER_MAJOR), SEMVER_MINOR=$(SEMVER_MINOR), SEMVER_PATCH=$(SEMVER_PATCH)
	$(MAKE) _prepare-syso-helper NAME="Jaeger Agent"            PKGPATH="cmd/agent"
	false
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Collector"        PKGPATH="cmd/collector"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Query"            PKGPATH="cmd/query"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Ingester"         PKGPATH="cmd/ingester"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Remote Storage"   PKGPATH="cmd/remote-storage"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger All-In-One"       PKGPATH="cmd/all-in-one"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Tracegen"         PKGPATH="cmd/tracegen"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger Anonymizer"       PKGPATH="cmd/anonymizer"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger ES-Index-Cleaner" PKGPATH="cmd/es-index-cleaner"
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger ES-Rollover"      PKGPATH="cmd/es-rollover"
	# TODO in the future this should be in v2
	$(MAKE) -e _prepare-syso-helper NAME="Jaeger V2"               PKGPATH="cmd/jaeger"

.PHONY: _clean-syso
_clean-syso:
	rm ./cmd/*/$(SYSOFILE)

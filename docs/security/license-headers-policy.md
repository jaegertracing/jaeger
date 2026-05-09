# License Header Policy

All in-scope source files must include both of the following near the top of the file:

- `Copyright (c) <year> The Jaeger Authors.`
- `SPDX-License-Identifier: Apache-2.0`

## In-Scope Files

The repository enforces headers for:

- `*.go`
- `*.py`
- `*.sh`
- `*.js`
- `*.ts`
- `*.yaml`
- `*.yml`
- `*.mk`
- `Makefile*`
- `Dockerfile*`

## Exclusions

The checker excludes:

- submodules and vendored code (`jaeger-ui`, `idl`, `vendor`)
- generated protobuf output (`*.pb.go`)
- generated/mock paths (`*/mocks/*`, `mocks*`)
- internal tool dependencies (`internal/tools`)
- docker debug script area (`scripts/build/docker/debug`)
- hidden and underscore-prefixed files (`.*`, `_*`)

## Enforcement

Local and CI enforcement is done by:

- `scripts/lint/license_headers.py` (autofix/check implementation)
- `make fmt` (adds missing headers)
- `make lint` target `lint-license` (fails if headers are missing)
- `.github/workflows/ci-lint-checks.yaml` via `make lint`

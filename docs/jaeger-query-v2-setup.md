# Jaeger Query v2 Setup Guide

## Overview
This guide explains the integration of Jaeger Query v2 into the Jaeger project. It includes details on the updated configuration, CI workflow, and how to use the new setup effectively.

## What Has Changed?
-  Jaeger Query v2 Support: Integrated into the CI/CD pipeline.
- Updated `ci-crossdock.yml` : Includes a step for running Jaeger Query v2.

## How to Use the New Configuration
### 1. Configuration File
- The configuration file is located at `cmd/config-query.yml`.
- Key changes include:
  - Support for in-memory storage.
  - Prometheus metrics configuration.
  - UI static files and index file setup.

### 2. Running Jaeger Query v2
- Use the following command to run Jaeger Query v2:
  ```bash
  docker run -d \
    --name jaeger-query-v2 \
    --rm \
    -p 16686:16686 \ # UI port
    -p 16685:16685 \ # Query service port
    -v ./cmd/jaeger/config.yaml:/etc/jaeger/config.yaml \
    jaegertracing/all-in-one:latest \
    --query.base-path=/jaeger
  ```

### 3. CI Workflow Integration
The CI workflow (`ci-crossdock.yml`) has been updated to:
- Include a step for running Jaeger Query v2 during testing.
- Ensure tests run against the new configuration.

## Additional Notes
- For more details, refer to the [`README.md`](/docs/jaeger-query-v2-setup.md) and [`CONTRIBUTING.md`](CONTRIBUTING.md) files as per the Jaeger documentation.

<!-- ## Troubleshooting
- If you encounter issues with the configuration, verify that the `config-query.yml` file is correctly mounted in the container.
- Ensure that the required ports (`16686` and `16685`) are not in use by other services. -->

# Troubleshooting Jaeger Query v2

Jaeger Query v2 is not starting?
- Check that `config-query.yml` is correctly mounted in the container.
- Ensure required ports (`16686`, `16685`) are available.

No traces appearing in the UI?
- Verify the **storage backend configuration** in `config-query.yml`.
- Confirm that your application is sending traces to Jaeger.

CI/CD workflow failing?
- Run the following command to inspect logs for errors:
  ```bash
  docker logs jaeger-query-v2

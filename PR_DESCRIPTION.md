# PR Description

Changes include several fixes, configuration updates, and new deployments for the OTEL demo environment.

### 🚀 New Deployments
- **Spark Dependencies:** Added a new CronJob (`spark-dependencies-cronjob-opensearch.yaml`) to calculate dependencies for OpenSearch storage.
- **HotROD:** Added a standalone `hotrod.yaml` deployment manifest to better control the HotROD application.

### 🔧 Configuration Updates
- **Ingress:**
    - Corrected the `jaeger-hotrod` service port to `8080`.
    - Split TLS certificates into separate secrets (`jaeger-ui-only-tls` and `hotrod-ui-only-tls`) to avoid rate-limiting or certificate management issues.
- **Jaeger:**
    - Enabled `create_mappings: true` for OpenSearch storage to ensure correct index setup.
    - Updated OTLP endpoints in `jaeger-values.yaml` to point to the correct `jaeger` service instead of `jaeger-collector`.
    - Standardized `imagePullPolicy` to `Always`.
- **OpenTelemetry Demo:**
    - Updated the Collector exporter to point to the `jaeger` service.
    - Explicitly set `OTEL_SERVICE_NAME` for the `frontend` component to `otelstore-frontend-ui`.
    - Simplified `OTEL_RESOURCE_ATTRIBUTES` configuration.

### 📦 Upgrades
- **OpenSearch Dashboards:** Upgraded image tag from `2.11.0` to `3.3.0` to match the backend version.

### 🛠️ Scripts
- **deploy-all.sh:**
    - Added a step to deploy and immediately trigger the Spark dependencies job.
    - Updated service readiness checks to wait for the proper `jaeger` service.

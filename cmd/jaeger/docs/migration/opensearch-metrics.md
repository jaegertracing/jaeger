# TITLE
### Combined Metrics

| V1 Metric | V1 Parameters | V2 Metric | V2 Parameters |
|-----------|---------------|-----------|---------------|
| jaeger_bulk_index_attempts_total | N/A | jaeger_bulk_index_attempts_total | N/A |
| jaeger_bulk_index_errors_total | N/A | jaeger_bulk_index_errors_total | N/A |
| jaeger_bulk_index_inserts_total | N/A | jaeger_bulk_index_inserts_total | N/A |
| jaeger_bulk_index_latency_err | N/A | jaeger_bulk_index_latency_err | N/A |
| jaeger_bulk_index_latency_ok | N/A | jaeger_bulk_index_latency_ok | N/A |
| jaeger_index_create_attempts_total | N/A | jaeger_index_create_attempts_total | N/A |
| jaeger_index_create_errors_total | N/A | jaeger_index_create_errors_total | N/A |
| jaeger_index_create_inserts_total | N/A | jaeger_index_create_inserts_total | N/A |
| jaeger_index_create_latency_err | N/A | jaeger_index_create_latency_err | N/A |
| jaeger_index_create_latency_ok | N/A | jaeger_index_create_latency_ok | N/A |
### Equivalent Metrics

| V1 Metric | V1 Parameters | V2 Metric | V2 Parameters |
|-----------|---------------|-----------|---------------|
| jaeger_collector_spans_rejected_total | debug, format, svc, transport | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| jaeger_build_info | build_date, revision,  version | target_info | service_instance_id, service_name, service_version |

# CASSANDRA METRICS
### Combined Metrics

| V1 Metric | V1 Parameters | V2 Metric | V2 Parameters |
|-----------|---------------|-----------|---------------|
| jaeger_cassandra_attempts_total | table | jaeger_cassandra_attempts_total | table |
| jaeger_cassandra_errors_total | table | jaeger_cassandra_errors_total | table |
| jaeger_cassandra_inserts_total | table | jaeger_cassandra_inserts_total | table |
| jaeger_cassandra_latency_err | table | jaeger_cassandra_latency_err | table |
| jaeger_cassandra_latency_ok | table | jaeger_cassandra_latency_ok | table |
| jaeger_cassandra_read_attempts_total | table | jaeger_cassandra_read_attempts_total | table |
| jaeger_cassandra_read_errors_total | table | jaeger_cassandra_read_errors_total | table |
| jaeger_cassandra_read_inserts_total | table | jaeger_cassandra_read_inserts_total | table |
| jaeger_cassandra_read_latency_err | table | jaeger_cassandra_read_latency_err | table |
| jaeger_cassandra_read_latency_ok | table | jaeger_cassandra_read_latency_ok | table |
| jaeger_cassandra_tag_index_skipped_total | N/A | jaeger_cassandra_tag_index_skipped_total | N/A |
### Equivalent Metrics

| V1 Metric | V1 Parameters | V2 Metric | V2 Parameters |
|-----------|---------------|-----------|---------------|
| jaeger_collector_spans_rejected_total | debug, format, svc, transport | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| jaeger_build_info | build_date, revision,  version | target_info | service_instance_id, service_name, service_version |

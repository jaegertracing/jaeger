# ALL-IN-ONE METRICS
### Combined Metrics

| V1 Metric | V1 Labels | V2 Metric | V2 Labels |
|-----------|---------------|-----------|---------------|
| jaeger_query_latency | operation, result | jaeger_query_latency | operation, result |
| jaeger_query_responses | operation | jaeger_query_responses | operation |
| jaeger_query_requests_total | operation, result | jaeger_query_requests_total | operation, result |
| go_gc_duration_seconds | N/A | N/A | N/A |
| go_goroutines | N/A | N/A | N/A |
| go_info | version | N/A | N/A |
| go_memstats_alloc_bytes | N/A | N/A | N/A |
| go_memstats_alloc_bytes_total | N/A | N/A | N/A |
| go_memstats_buck_hash_sys_bytes | N/A | N/A | N/A |
| go_memstats_frees_total | N/A | N/A | N/A |
| go_memstats_gc_sys_bytes | N/A | N/A | N/A |
| go_memstats_heap_alloc_bytes | N/A | N/A | N/A |
| go_memstats_heap_idle_bytes | N/A | N/A | N/A |
| go_memstats_heap_inuse_bytes | N/A | N/A | N/A |
| go_memstats_heap_objects | N/A | N/A | N/A |
| go_memstats_heap_released_bytes | N/A | N/A | N/A |
| go_memstats_heap_sys_bytes | N/A | N/A | N/A |
| go_memstats_last_gc_time_seconds | N/A | N/A | N/A |
| go_memstats_lookups_total | N/A | N/A | N/A |
| go_memstats_mallocs_total | N/A | N/A | N/A |
| go_memstats_mcache_inuse_bytes | N/A | N/A | N/A |
| go_memstats_mcache_sys_bytes | N/A | N/A | N/A |
| go_memstats_mspan_inuse_bytes | N/A | N/A | N/A |
| go_memstats_mspan_sys_bytes | N/A | N/A | N/A |
| go_memstats_next_gc_bytes | N/A | N/A | N/A |
| go_memstats_other_sys_bytes | N/A | N/A | N/A |
| go_memstats_stack_inuse_bytes | N/A | N/A | N/A |
| go_memstats_stack_sys_bytes | N/A | N/A | N/A |
| go_memstats_sys_bytes | N/A | N/A | N/A |
| go_threads | N/A | N/A | N/A |
| jaeger_build_info | build_date, revision, version | N/A | N/A |
| jaeger_collector_batch_size | host | N/A | N/A |
| jaeger_collector_http_request_duration | method, path, status | N/A | N/A |
| jaeger_collector_http_server_errors_total | source, status | N/A | N/A |
| jaeger_collector_http_server_requests_total | type | N/A | N/A |
| jaeger_collector_in_queue_latency | host | N/A | N/A |
| jaeger_collector_queue_capacity | host | N/A | N/A |
| jaeger_collector_queue_length | host | N/A | N/A |
| jaeger_collector_save_latency | host | N/A | N/A |
| jaeger_collector_spans_bytes | host | N/A | N/A |
| jaeger_collector_spans_dropped_total | host | N/A | N/A |
| jaeger_collector_spans_received_total | debug, format, svc, transport | N/A | N/A |
| jaeger_collector_spans_rejected_total | debug, format, svc, transport | N/A | N/A |
| jaeger_collector_spans_saved_by_svc_total | debug, result, svc | N/A | N/A |
| jaeger_collector_spans_serviceNames | host | N/A | N/A |
| jaeger_collector_traces_received_total | debug, format, sampler_type, svc, transport | N/A | N/A |
| jaeger_collector_traces_rejected_total | debug, format, sampler_type, svc, transport | N/A | N/A |
| jaeger_collector_traces_saved_by_svc_total | debug, result, sampler_type, svc | N/A | N/A |
| process_cpu_seconds_total | N/A | N/A | N/A |
| process_max_fds | N/A | N/A | N/A |
| process_open_fds | N/A | N/A | N/A |
| process_resident_memory_bytes | N/A | N/A | N/A |
| process_start_time_seconds | N/A | N/A | N/A |
| process_virtual_memory_bytes | N/A | N/A | N/A |
| process_virtual_memory_max_bytes | N/A | N/A | N/A |
| N/A | N/A | exporter_send_failed_spans | exporter, service_instance_id, service_name, service_version |
| N/A | N/A | exporter_sent_spans | exporter, service_instance_id, service_name, service_version |
| N/A | N/A | process_cpu_seconds | service_instance_id, service_name, service_version |
| N/A | N/A | process_memory_rss | service_instance_id, service_name, service_version |
| N/A | N/A | process_runtime_heap_alloc_bytes | service_instance_id, service_name, service_version |
| N/A | N/A | process_runtime_total_alloc_bytes | service_instance_id, service_name, service_version |
| N/A | N/A | process_runtime_total_sys_memory_bytes | service_instance_id, service_name, service_version |
| N/A | N/A | process_uptime | service_instance_id, service_name, service_version |
| N/A | N/A | processor_batch_batch_send_size | processor, service_instance_id, service_name, service_version |
| N/A | N/A | processor_batch_batch_send_size_bytes | processor, service_instance_id, service_name, service_version |
| N/A | N/A | processor_batch_metadata_cardinality | processor, service_instance_id, service_name, service_version |
| N/A | N/A | processor_batch_timeout_trigger_send | processor, service_instance_id, service_name, service_version |
| N/A | N/A | receiver_accepted_spans | receiver, service_instance_id, service_name, service_version, transport |
| N/A | N/A | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| N/A | N/A | rpc_server_duration | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| N/A | N/A | rpc_server_request_size | rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| N/A | N/A | rpc_server_requests_per_rpc | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| N/A | N/A | rpc_server_response_size | rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| N/A | N/A | rpc_server_responses_per_rpc | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| N/A | N/A | target_info | service_instance_id, service_name, service_version |
### Equivalent Metrics

| V1 Metric | V1 Labels | V2 Metric | V2 Labels |
|-----------|---------------|-----------|---------------|
| jaeger_collector_spans_rejected_total | debug, format, svc, transport | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| jaeger_build_info | build_date, revision,  version | target_info | service_instance_id, service_name, service_version |

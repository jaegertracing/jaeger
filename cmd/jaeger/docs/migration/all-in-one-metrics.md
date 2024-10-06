### Common Metrics

| Metric Name | Inner Parameters |
|-------------|------------------|
| jaeger_query_latency | operation, result |
| jaeger_query_responses | operation |
| jaeger_query_requests_total | operation, result |
### V1 Only Metrics

| Metric Name | Inner Parameters |
|-------------|------------------|
| go_gc_duration_seconds |  |
| go_goroutines |  |
| go_info | version |
| go_memstats_alloc_bytes |  |
| go_memstats_alloc_bytes_total |  |
| go_memstats_buck_hash_sys_bytes |  |
| go_memstats_frees_total |  |
| go_memstats_gc_sys_bytes |  |
| go_memstats_heap_alloc_bytes |  |
| go_memstats_heap_idle_bytes |  |
| go_memstats_heap_inuse_bytes |  |
| go_memstats_heap_objects |  |
| go_memstats_heap_released_bytes |  |
| go_memstats_heap_sys_bytes |  |
| go_memstats_last_gc_time_seconds |  |
| go_memstats_lookups_total |  |
| go_memstats_mallocs_total |  |
| go_memstats_mcache_inuse_bytes |  |
| go_memstats_mcache_sys_bytes |  |
| go_memstats_mspan_inuse_bytes |  |
| go_memstats_mspan_sys_bytes |  |
| go_memstats_next_gc_bytes |  |
| go_memstats_other_sys_bytes |  |
| go_memstats_stack_inuse_bytes |  |
| go_memstats_stack_sys_bytes |  |
| go_memstats_sys_bytes |  |
| go_threads |  |
| jaeger_build_info | build_date, revision, version |
| jaeger_collector_batch_size | host |
| jaeger_collector_http_request_duration | method, path, status |
| jaeger_collector_http_server_errors_total | source, status |
| jaeger_collector_http_server_requests_total | type |
| jaeger_collector_in_queue_latency | host |
| jaeger_collector_queue_capacity | host |
| jaeger_collector_queue_length | host |
| jaeger_collector_save_latency | host |
| jaeger_collector_spans_bytes | host |
| jaeger_collector_spans_dropped_total | host |
| jaeger_collector_spans_received_total | debug, format, svc, transport |
| jaeger_collector_spans_rejected_total | debug, format, svc, transport |
| jaeger_collector_spans_saved_by_svc_total | debug, result, svc |
| jaeger_collector_spans_serviceNames | host |
| jaeger_collector_traces_received_total | debug, format, sampler_type, svc, transport |
| jaeger_collector_traces_rejected_total | debug, format, sampler_type, svc, transport |
| jaeger_collector_traces_saved_by_svc_total | debug, result, sampler_type, svc |
| process_cpu_seconds_total |  |
| process_max_fds |  |
| process_open_fds |  |
| process_resident_memory_bytes |  |
| process_start_time_seconds |  |
| process_virtual_memory_bytes |  |
| process_virtual_memory_max_bytes |  |
### V2 Only Metrics

| Metric Name | Inner Parameters |
|-------------|------------------|
| exporter_send_failed_spans | exporter, service_instance_id, service_name, service_version |
| exporter_sent_spans | exporter, service_instance_id, service_name, service_version |
| process_cpu_seconds | service_instance_id, service_name, service_version |
| process_memory_rss | service_instance_id, service_name, service_version |
| process_runtime_heap_alloc_bytes | service_instance_id, service_name, service_version |
| process_runtime_total_alloc_bytes | service_instance_id, service_name, service_version |
| process_runtime_total_sys_memory_bytes | service_instance_id, service_name, service_version |
| process_uptime | service_instance_id, service_name, service_version |
| processor_batch_batch_send_size | processor, service_instance_id, service_name, service_version |
| processor_batch_batch_send_size_bytes | processor, service_instance_id, service_name, service_version |
| processor_batch_metadata_cardinality | processor, service_instance_id, service_name, service_version |
| processor_batch_timeout_trigger_send | processor, service_instance_id, service_name, service_version |
| receiver_accepted_spans | receiver, service_instance_id, service_name, service_version, transport |
| receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| rpc_server_duration | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| rpc_server_request_size | rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| rpc_server_requests_per_rpc | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| rpc_server_response_size | rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| rpc_server_responses_per_rpc | rpc_grpc_status_code, rpc_method, rpc_service, rpc_system, service_instance_id, service_name, service_version |
| target_info | service_instance_id, service_name, service_version |
### Equivalent Metrics

| V1 Metric | V1 Parameters | V2 Metric | V2 Parameters |
|-----------|---------------|-----------|---------------|
| jaeger_collector_spans_rejected_total | debug, format, svc, transport | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |

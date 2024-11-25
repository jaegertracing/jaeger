# BADGER METRICS

### Combined Metrics

| V1 Metric                                  | V1 Parameters | V2 Metric                                | V2 Parameters |
| ------------------------------------------ | ------------- | ---------------------------------------- | ------------- |
| jaeger_badger_compaction_current_num_lsm   | N/A           | jaeger_badger_compaction_current_num_lsm | N/A           |
| jaeger_badger_get_num_memtable             | N/A           | jaeger_badger_get_num_memtable           | N/A           |
| jaeger_badger_get_num_user                 | N/A           | jaeger_badger_get_num_user               | N/A           |
| jaeger_badger_get_with_result_num_user     | N/A           | jaeger_badger_get_with_result_num_user   | N/A           |
| jaeger_badger_iterator_num_user            | N/A           | jaeger_badger_iterator_num_user          | N/A           |
| jaeger_badger_put_num_user                 | N/A           | jaeger_badger_put_num_user               | N/A           |
| jaeger_badger_read_bytes_lsm               | N/A           | jaeger_badger_read_bytes_lsm             | N/A           |
| jaeger_badger_read_bytes_vlog              | N/A           | jaeger_badger_read_bytes_vlog            | N/A           |
| jaeger_badger_read_num_vlog                | N/A           | jaeger_badger_read_num_vlog              | N/A           |
| jaeger_badger_size_bytes_lsm               | N/A           | jaeger_badger_size_bytes_lsm             | N/A           |
| jaeger_badger_size_bytes_vlog              | N/A           | jaeger_badger_size_bytes_vlog            | N/A           |
| jaeger_badger_write_bytes_l0               | N/A           | jaeger_badger_write_bytes_l0             | N/A           |
| jaeger_badger_write_bytes_user             | N/A           | jaeger_badger_write_bytes_user           | N/A           |
| jaeger_badger_write_bytes_vlog             | N/A           | jaeger_badger_write_bytes_vlog           | N/A           |
| jaeger_badger_write_num_vlog               | N/A           | jaeger_badger_write_num_vlog             | N/A           |
| jaeger_badger_write_pending_num_memtable   | N/A           | jaeger_badger_write_pending_num_memtable | N/A           |
| jaeger_badger_key_log_bytes_available      | N/A           | N/A                                      | N/A           |
| jaeger_badger_storage_maintenance_last_run | N/A           | N/A                                      | N/A           |
| jaeger_badger_storage_valueloggc_last_run  | N/A           | N/A                                      | N/A           |
| jaeger_badger_value_log_bytes_available    | N/A           | N/A                                      | N/A           |

### Equivalent Metrics

| V1 Metric                             | V1 Parameters                  | V2 Metric              | V2 Parameters                                                           |
| ------------------------------------- | ------------------------------ | ---------------------- | ----------------------------------------------------------------------- |
| jaeger_collector_spans_rejected_total | debug, format, svc, transport  | receiver_refused_spans | receiver, service_instance_id, service_name, service_version, transport |
| jaeger_build_info                     | build_date, revision,  version | target_info            | service_instance_id, service_name, service_version                      |


/**
 * The traces table holds basic information about the trace
 */
CREATE TABLE IF NOT EXISTS traces (
  trace_id BLOB,
  span_id  BIGINT,
  parent_id BIGINT,
  operation_name TEXT,
  flags INT,
  start_time BIGINT,
  duration BIGINT,
  service_name TEXT,
  PRIMARY KEY (trace_id, span_id)
);

/**
 * This index is needed so we can easily list all services or traces for a specific service
 */
CREATE INDEX IF NOT EXISTS service_name ON traces(service_name);
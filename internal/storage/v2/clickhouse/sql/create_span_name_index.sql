ALTER TABLE spans
ADD INDEX idx_span_name name TYPE set(0) GRANULARITY 1;
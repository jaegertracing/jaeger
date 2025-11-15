ALTER TABLE spans
ADD INDEX idx_service_name service_name TYPE set(0) GRANULARITY 1;
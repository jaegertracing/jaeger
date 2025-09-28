CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations AS
SELECT DISTINCT
    name,
    kind AS span_kind,
    service_name
FROM
    spans;
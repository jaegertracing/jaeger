-- operations materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations_local AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM
    spans_local;

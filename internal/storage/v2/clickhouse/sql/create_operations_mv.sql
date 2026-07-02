CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM
    spans
GROUP BY
    service_name,
    name,
    kind;

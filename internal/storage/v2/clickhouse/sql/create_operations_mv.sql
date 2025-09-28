CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations AS
SELECT
    name,
    kind AS span_kind
FROM
    spans
GROUP BY
    name,
    span_kind
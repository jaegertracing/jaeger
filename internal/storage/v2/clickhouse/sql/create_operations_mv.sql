CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations{{ .Suffix }} AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM
    spans{{ .Suffix }};

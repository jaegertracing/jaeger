CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations{{ .LocalTableSuffix }} AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM
    spans{{ .LocalTableSuffix }};

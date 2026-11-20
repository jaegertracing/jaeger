CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv TO services{{ .LocalTableSuffix }} AS
SELECT
    service_name AS name
FROM
    spans{{ .LocalTableSuffix }}
GROUP BY
    service_name;

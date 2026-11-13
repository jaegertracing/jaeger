CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv TO services{{ .Suffix }} AS
SELECT
    service_name AS name
FROM
    spans{{ .Suffix }}
GROUP BY
    service_name;

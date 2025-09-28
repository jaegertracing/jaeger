CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv TO services AS
SELECT
    service_name AS name
FROM
    spans
GROUP BY
    service_name
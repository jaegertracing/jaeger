-- services materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv TO services_local AS
SELECT
    service_name AS name
FROM
    spans_local
GROUP BY
    service_name;

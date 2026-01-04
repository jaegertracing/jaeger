SELECT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
GROUP BY name, span_kind
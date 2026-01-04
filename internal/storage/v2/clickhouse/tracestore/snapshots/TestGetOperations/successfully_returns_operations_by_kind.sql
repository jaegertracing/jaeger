SELECT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
    AND span_kind = ?
GROUP BY name, span_kind
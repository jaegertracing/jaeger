CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    attribute_key,
    type
FROM
    (
        SELECT
            arrayJoin(arrayConcat(bool_attributes.key, resource_bool_attributes.key)) AS attribute_key,
            'bool' AS type
        FROM
            spans
        WHERE
            length(bool_attributes.key) > 0 OR length(resource_bool_attributes.key) > 0
        UNION ALL
        SELECT
            arrayJoin(arrayConcat(double_attributes.key, resource_double_attributes.key)) AS attribute_key,
            'double' AS type
        FROM
            spans
        WHERE
            length(double_attributes.key) > 0 OR length(resource_double_attributes.key) > 0
        UNION ALL
        SELECT
            arrayJoin(arrayConcat(int_attributes.key, resource_int_attributes.key)) AS attribute_key,
            'int' AS type
        FROM
            spans
        WHERE
            length(int_attributes.key) > 0 OR length(resource_int_attributes.key) > 0
        UNION ALL
        SELECT
            arrayJoin(arrayConcat(str_attributes.key, resource_str_attributes.key)) AS attribute_key,
            'str' AS type
        FROM
            spans
        WHERE
            length(str_attributes.key) > 0 OR length(resource_str_attributes.key) > 0
        UNION ALL
        SELECT
            CASE
                WHEN startsWith(key, '@bytes@') THEN substring(key, 8)
                WHEN startsWith(key, '@map@') THEN substring(key, 6)
                WHEN startsWith(key, '@slice@') THEN substring(key, 8)
                ELSE key
            END AS attribute_key,
            CASE
                WHEN startsWith(key, '@bytes@') THEN 'bytes'
                WHEN startsWith(key, '@map@') THEN 'map'
                WHEN startsWith(key, '@slice@') THEN 'slice'
                ELSE ''
            END AS type
        FROM
            (
                SELECT
                    arrayJoin(arrayConcat(complex_attributes.key, resource_complex_attributes.key)) AS key
                FROM
                    spans
                WHERE
                    length(complex_attributes.key) > 0 OR length(resource_complex_attributes.key) > 0
            )
    )
GROUP BY
    attribute_key,
    type;

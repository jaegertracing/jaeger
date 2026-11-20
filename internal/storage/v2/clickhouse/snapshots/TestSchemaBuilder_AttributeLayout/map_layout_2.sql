CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     mapKeys(bool_attributes)),
            arrayMap(k -> (k, 'bool',   'resource'), mapKeys(resource_bool_attributes)),
            arrayMap(k -> (k, 'bool',   'scope'),    mapKeys(scope_bool_attributes)),
            arrayMap(k -> (k, 'double', 'span'),     mapKeys(double_attributes)),
            arrayMap(k -> (k, 'double', 'resource'), mapKeys(resource_double_attributes)),
            arrayMap(k -> (k, 'double', 'scope'),    mapKeys(scope_double_attributes)),
            arrayMap(k -> (k, 'int',    'span'),     mapKeys(int_attributes)),
            arrayMap(k -> (k, 'int',    'resource'), mapKeys(resource_int_attributes)),
            arrayMap(k -> (k, 'int',    'scope'),    mapKeys(scope_int_attributes)),
            arrayMap(k -> (k, 'str',    'span'),     mapKeys(str_attributes)),
            arrayMap(k -> (k, 'str',    'resource'), mapKeys(resource_str_attributes)),
            arrayMap(k -> (k, 'str',    'scope'),    mapKeys(scope_str_attributes)),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), mapKeys(complex_attributes)),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), mapKeys(resource_complex_attributes)),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), mapKeys(scope_complex_attributes))
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;

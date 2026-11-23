CREATE TABLE
    IF NOT EXISTS services{{ .LocalTableSuffix }} (name String) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY
    (name);

CREATE TABLE
    IF NOT EXISTS services{{ .Suffix }} (name String) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY
    (name);

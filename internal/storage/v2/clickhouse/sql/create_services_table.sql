CREATE TABLE
    IF NOT EXISTS services (name String) ENGINE = ReplacingMergeTree
ORDER BY
    (name);
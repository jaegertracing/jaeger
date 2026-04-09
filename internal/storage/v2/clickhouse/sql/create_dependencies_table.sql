CREATE TABLE
    IF NOT EXISTS dependencies (
        timestamp DateTime64 (9),
        dependencies String
    ) ENGINE = MergeTree
PARTITION BY
    toDate(timestamp)
ORDER BY
    (timestamp);
-- dependencies table
CREATE TABLE
    IF NOT EXISTS dependencies_local (
        timestamp DateTime64 (9),
        dependencies_json String
    ) ENGINE = ReplicatedMergeTree
PARTITION BY
    toDate(timestamp)
ORDER BY
    (timestamp);

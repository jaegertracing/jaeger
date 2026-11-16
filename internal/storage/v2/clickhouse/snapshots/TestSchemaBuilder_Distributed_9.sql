-- trace id timestamps table
CREATE TABLE IF NOT EXISTS trace_id_timestamps_local
(
    trace_id String,
    start SimpleAggregateFunction(min, DateTime64(9)),
    end SimpleAggregateFunction(max, DateTime64(9))
)
ENGINE = ReplicatedAggregatingMergeTree
ORDER BY (trace_id)

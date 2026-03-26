CREATE TABLE IF NOT EXISTS trace_id_timestamps
(
    trace_id String,
    start SimpleAggregateFunction(min, DateTime64(9)),
    end SimpleAggregateFunction(max, DateTime64(9))
)
ENGINE = AggregatingMergeTree()
ORDER BY (trace_id);
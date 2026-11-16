-- trace_id_timestamps distributed table
CREATE TABLE
    IF NOT EXISTS trace_id_timestamps AS trace_id_timestamps_local ENGINE = Distributed(
        'my_cluster',
        currentDatabase(),
        'trace_id_timestamps_local',
        cityHash64(trace_id)
    )

CREATE TABLE
    IF NOT EXISTS spans AS spans_local ENGINE = Distributed(
        'my_cluster',
        'jaeger',
        'spans_local',
        cityHash64(trace_id)
    )

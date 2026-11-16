-- dependencies distributed table
CREATE TABLE
    IF NOT EXISTS dependencies AS dependencies_local ENGINE = Distributed(
        'my_cluster',
        currentDatabase(),
        'dependencies_local',
        cityHash64(timestamp)
    )

-- operations distributed table
CREATE TABLE
    IF NOT EXISTS operations AS operations_local ENGINE = Distributed(
        'my_cluster',
        currentDatabase(),
        'operations_local'
    )

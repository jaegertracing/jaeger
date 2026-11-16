-- services distributed table
CREATE TABLE
    IF NOT EXISTS services AS services_local ENGINE = Distributed(
        'my_cluster',
        currentDatabase(),
        'services_local'
    )

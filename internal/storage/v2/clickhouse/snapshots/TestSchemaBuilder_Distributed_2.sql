CREATE TABLE
    IF NOT EXISTS services AS services_local ENGINE = Distributed(
        'my_cluster',
        'jaeger',
        'services_local'
    )

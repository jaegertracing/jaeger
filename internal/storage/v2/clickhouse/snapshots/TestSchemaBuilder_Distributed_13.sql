-- attribute metadata distributed table
CREATE TABLE
    IF NOT EXISTS attribute_metadata AS attribute_metadata_local ENGINE = Distributed(
        'my_cluster',
        currentDatabase(),
        'attribute_metadata_local'
    )

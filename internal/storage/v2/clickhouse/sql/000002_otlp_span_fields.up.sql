ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS flags UInt32,
    ADD COLUMN IF NOT EXISTS dropped_attributes_count UInt32,
    ADD COLUMN IF NOT EXISTS dropped_events_count UInt32,
    ADD COLUMN IF NOT EXISTS dropped_links_count UInt32,
    ADD COLUMN IF NOT EXISTS events.dropped_attributes_count Array(UInt32) DEFAULT arrayMap(x -> toUInt32(0), events.name),
    ADD COLUMN IF NOT EXISTS links.dropped_attributes_count Array(UInt32) DEFAULT arrayMap(x -> toUInt32(0), links.trace_id),
    ADD COLUMN IF NOT EXISTS links.flags Array(UInt32) DEFAULT arrayMap(x -> toUInt32(0), links.trace_id),
    ADD COLUMN IF NOT EXISTS resource_schema_url String,
    ADD COLUMN IF NOT EXISTS scope_schema_url String,
    ADD COLUMN IF NOT EXISTS scope_dropped_attributes_count UInt32;

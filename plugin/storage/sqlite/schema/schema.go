package schema

const InitSQL = `
-- table to store span data
CREATE TABLE IF NOT EXISTS spans (
    trace_id TEXT,
    span_id TEXT,
    parent_id TEXT,
    service_name TEXT,
    operation_name TEXT,
    start_time DATETIME,
    duration INT,
    span BLOB,
    PRIMARY KEY (trace_id, span_id)
);

-- index spans by trace_id
CREATE INDEX IF NOT EXISTS SPANS_BY_TRACE_ID_IDX ON spans(trace_id);

-- index spans by service_name
CREATE INDEX IF NOT EXISTS SPANS_BY_SERVICE_NAME_IDX ON spans(service_name);

-- index spans by service_name, start_time
CREATE INDEX IF NOT EXISTS SPANS_BY_SERVICE_NAME_START_TIME_IDX ON spans(service_name, start_time);

-- index spans by service_name, operation_name, start_time, duration
CREATE INDEX IF NOT EXISTS SPANS_BY_SERVICE_NAME_OPERATION_NAME_START_TIME_IDX ON spans(service_name, operation_name, start_time, duration);

-- table to store span tags
CREATE TABLE IF NOT EXISTS span_tags (
    trace_id TEXT,
    span_id TEXT,
    tag TEXT,
    PRIMARY KEY (trace_id, span_id, tag),
    CONSTRAINT FK FOREIGN KEY (trace_id, span_id) REFERENCES spans (trace_id, span_id) ON DELETE CASCADE
);

-- index spans by tag
CREATE INDEX IF NOT EXISTS SPANS_BY_TAG on span_tags (tag);
`

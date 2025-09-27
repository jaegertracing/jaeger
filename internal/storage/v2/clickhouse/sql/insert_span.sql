INSERT INTO
    spans (
        id,
        trace_id,
        trace_state,
        parent_span_id,
        name,
        kind,
        start_time,
        status_code,
        status_message,
        duration,
        service_name,
        scope_name,
        scope_version
    )
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
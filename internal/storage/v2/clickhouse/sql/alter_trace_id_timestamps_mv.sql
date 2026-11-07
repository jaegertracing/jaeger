ALTER TABLE trace_id_timestamps_mv
MODIFY QUERY
SELECT
    trace_id,
    min(start_time) AS start,
    max(addNanoseconds(start_time, duration)) AS end
FROM spans
GROUP BY trace_id;

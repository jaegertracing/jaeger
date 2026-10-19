SELECT
    trace_id,
    min(start) AS start,
    max(end) AS end
FROM trace_id_timestamps
WHERE trace_id IN (?)
GROUP BY trace_id

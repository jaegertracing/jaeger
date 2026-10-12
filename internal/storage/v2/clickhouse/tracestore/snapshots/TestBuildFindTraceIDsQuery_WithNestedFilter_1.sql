SELECT
    l.trace_id,
    min(t.start) AS start,
    max(t.end) AS end
FROM (
	SELECT DISTINCT
	    s.trace_id
	FROM spans s
	WHERE 1=1
		AND (
			(
				(
					s.name = ?
					OR
					(
						arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
					)
				)
				AND
				NOT (
					s.duration > ?
				)
			)
		)
		AND s.start_time >= ?
		AND s.start_time <= ?
	LIMIT ?
) l
LEFT JOIN trace_id_timestamps t ON l.trace_id = t.trace_id
GROUP BY l.trace_id

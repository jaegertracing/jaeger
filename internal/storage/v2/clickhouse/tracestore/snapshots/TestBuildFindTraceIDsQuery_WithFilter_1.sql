SELECT DISTINCT
    s.trace_id
FROM spans s
WHERE 1=1
	AND (
		s.service_name = ?
	)
	AND s.start_time >= ?
	AND s.start_time <= ?
LIMIT ?

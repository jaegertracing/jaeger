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
	AND s.start_time >= fromUnixTimestamp64Nano(?)
	AND s.start_time <= fromUnixTimestamp64Nano(?)
LIMIT ?

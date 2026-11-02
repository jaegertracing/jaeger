(
	(
		s.name = ?
		OR
		s.name = ?
	)
	AND
	NOT (
		s.service_name = ?
	)
)

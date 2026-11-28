(
	(
		s.name = ?
		OR
		s.name = ?
	)
	AND
	NOT (
		(s.service_name != '' AND s.service_name = ?)
	)
)

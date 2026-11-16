(
	(s.`model_column` IS NOT NULL AND s.`model_column` = ?)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
)

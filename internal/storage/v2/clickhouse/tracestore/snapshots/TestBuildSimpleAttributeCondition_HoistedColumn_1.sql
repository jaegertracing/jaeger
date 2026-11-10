(s.`model_column` IS NOT NULL AND s.`model_column` = ?)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_str_attributes.key, s.scope_str_attributes.value)
		OR
		arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.events)
		OR
		arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links)

(
	arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links)
)

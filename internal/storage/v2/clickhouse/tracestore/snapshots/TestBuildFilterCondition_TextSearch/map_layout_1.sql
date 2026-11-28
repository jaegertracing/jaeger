-- every listed word in a span attribute, with ASCII letters lowercased
-- binds [note note [premium refund Über]]
(
	(mapContains(s.str_attributes, ?) AND hasAllTokens(lower(s.str_attributes[?]), ?))
)
-- the listed words adjacent and in order in a span attribute, with ASCII letters lowercased
-- binds [note note [refund policy]]
(
	(mapContains(s.str_attributes, ?) AND hasSubstr(tokens(lower(s.str_attributes[?])), ?))
)
-- words in an unqualified attribute search the span and the resource
-- binds [note note [refund] note note [refund]]
(
	(mapContains(s.str_attributes, ?) AND hasAllTokens(lower(s.str_attributes[?]), ?))
	OR
	(mapContains(s.resource_str_attributes, ?) AND hasAllTokens(lower(s.resource_str_attributes[?]), ?))
)
-- a phrase in an event attribute
-- binds [exception.message exception.message [connection refused]]
(
	arrayExists(x -> mapContains(x.str_attributes, ?) AND hasSubstr(tokens(lower(x.str_attributes[?])), ?), s.events)
)
-- words in a link attribute
-- binds [note note [refund]]
(
	arrayExists(x -> mapContains(x.str_attributes, ?) AND hasAllTokens(lower(x.str_attributes[?]), ?), s.links)
)
-- a phrase in an attribute of the event that a some operator binds
-- binds [exception exception.message exception.message [connection refused]]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(
			(mapContains(x.str_attributes, ?) AND hasSubstr(tokens(lower(x.str_attributes[?])), ?))
		)
	)
), s.events)
-- a negated phrase also matches a span without the attribute
-- binds [note note [refund policy]]
NOT (
	(
		(mapContains(s.str_attributes, ?) AND hasSubstr(tokens(lower(s.str_attributes[?])), ?))
	)
)

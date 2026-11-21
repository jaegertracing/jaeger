-- two fields of one event
-- binds [exception 50000]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(toUnixTimestamp64Nano(x.timestamp) - toUnixTimestamp64Nano(s.start_time)) > ?
	)
), s.events)
-- a field and an attribute of one event
-- binds [exception exception.type exception.type IOError]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(
			(mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?)
		)
	)
), s.events)
-- the bound level beside the level of the span
-- binds [^cache GET /cart zone zone us-east]
arrayExists(x -> (
	(
		x.name REGEXP ?
		AND
		s.name = ?
		AND
		(
			(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
		)
	)
), s.events)
-- exists, ne and not_in on one link
-- binds [congo=t61rcWkgMzE zone zone zone zone us-east zone zone eu-west]
arrayExists(x -> (
	(
		x.trace_state != ''
		AND
		(x.trace_state != '' AND NOT (x.trace_state = ?))
		AND
		(
			(mapContains(x.str_attributes, ?))
		)
		AND
		(
			(
				(mapContains(x.str_attributes, ?))
			)
			AND NOT
			(
				(mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?)
				OR
				(mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?)
			)
		)
	)
), s.links)
-- an attribute of an event inside a predicate over one link searches every event
-- binds [00000000000000000000000000000f01 attempt attempt 1]
arrayExists(x -> (
	(
		x.trace_id = ?
		AND
		(
			arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] > ?, s.events)
		)
	)
), s.links)
-- every event is named, by a negation around a negated predicate
-- binds []
NOT (
	arrayExists(x -> (
		NOT (
			true
		)
	), s.events)
)
-- a level is bound only inside its own predicate
-- binds [exception retry]
(
	arrayExists(x -> (
		x.name = ?
	), s.events)
	AND
	arrayExists(x -> x.name = ?, s.events)
)

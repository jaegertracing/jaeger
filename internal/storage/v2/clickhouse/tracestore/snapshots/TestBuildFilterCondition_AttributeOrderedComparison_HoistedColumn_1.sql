-- an ordered comparison on a hoisted attribute
-- binds [7]
(
	(s.`shard_column` IS NOT NULL AND s.`shard_column` > ?)
)

-- a pattern on a hoisted attribute
-- binds [^claude]
(
	(s.`model_column` IS NOT NULL AND s.`model_column` REGEXP ?)
)

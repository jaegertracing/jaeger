-- words in a hoisted attribute
-- binds [[claude]]
(
	(s.`model_column` IS NOT NULL AND hasAllTokens(lower(s.`model_column`), ?))
)
-- a phrase in a hoisted attribute
-- binds [[claude fable]]
(
	(s.`model_column` IS NOT NULL AND hasSubstr(tokens(lower(assumeNotNull(s.`model_column`))), ?))
)

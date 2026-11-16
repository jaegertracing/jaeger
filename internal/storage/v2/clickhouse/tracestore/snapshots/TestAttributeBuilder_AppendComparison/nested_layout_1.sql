-- span attribute "model" of type Str, hoisted into the column "model_column"
-- binds [x]
(s.`model_column` IS NOT NULL AND s.`model_column` = ?)
-- span attribute "model" of type Int
-- binds [model 1]
arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
-- span attribute "shard" of type Int
-- binds [shard 0]
arrayExists((key, value) -> key = ? AND value >= ?, s.int_attributes.key, s.int_attributes.value)
-- span attribute "ratio" of type Double
-- binds [ratio 0.5]
arrayExists((key, value) -> key = ? AND value < ?, s.double_attributes.key, s.double_attributes.value)
-- span attribute "retry" of type Bool
-- binds [retry true]
arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)
-- resource attribute "model" of type Str
-- binds [model x]
arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
-- resource attribute "model" of type Int
-- binds [model 1]
arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
-- resource attribute "shard" of type Int, hoisted into the column "shard_column"
-- binds [0]
(s.`shard_column` IS NOT NULL AND s.`shard_column` >= ?)
-- resource attribute "ratio" of type Double
-- binds [ratio 0.5]
arrayExists((key, value) -> key = ? AND value < ?, s.resource_double_attributes.key, s.resource_double_attributes.value)
-- resource attribute "retry" of type Bool
-- binds [retry true]
arrayExists((key, value) -> key = ? AND value = ?, s.resource_bool_attributes.key, s.resource_bool_attributes.value)
-- scope attribute "model" of type Str
-- binds [model x]
arrayExists((key, value) -> key = ? AND value = ?, s.scope_str_attributes.key, s.scope_str_attributes.value)
-- scope attribute "model" of type Int
-- binds [model 1]
arrayExists((key, value) -> key = ? AND value = ?, s.scope_int_attributes.key, s.scope_int_attributes.value)
-- scope attribute "shard" of type Int
-- binds [shard 0]
arrayExists((key, value) -> key = ? AND value >= ?, s.scope_int_attributes.key, s.scope_int_attributes.value)
-- scope attribute "ratio" of type Double
-- binds [ratio 0.5]
arrayExists((key, value) -> key = ? AND value < ?, s.scope_double_attributes.key, s.scope_double_attributes.value)
-- scope attribute "retry" of type Bool
-- binds [retry true]
arrayExists((key, value) -> key = ? AND value = ?, s.scope_bool_attributes.key, s.scope_bool_attributes.value)
-- event attribute "model" of type Str
-- binds [model x]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.events)
-- event attribute "model" of type Int
-- binds [model 1]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.int_attributes.key, x.int_attributes.value), s.events)
-- event attribute "shard" of type Int
-- binds [shard 0]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value >= ?, x.int_attributes.key, x.int_attributes.value), s.events)
-- event attribute "ratio" of type Double
-- binds [ratio 0.5]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value < ?, x.double_attributes.key, x.double_attributes.value), s.events)
-- event attribute "retry" of type Bool
-- binds [retry true]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.bool_attributes.key, x.bool_attributes.value), s.events)
-- link attribute "model" of type Str
-- binds [model x]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links)
-- link attribute "model" of type Int
-- binds [model 1]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.int_attributes.key, x.int_attributes.value), s.links)
-- link attribute "shard" of type Int
-- binds [shard 0]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value >= ?, x.int_attributes.key, x.int_attributes.value), s.links)
-- link attribute "ratio" of type Double
-- binds [ratio 0.5]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value < ?, x.double_attributes.key, x.double_attributes.value), s.links)
-- link attribute "retry" of type Bool
-- binds [retry true]
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.bool_attributes.key, x.bool_attributes.value), s.links)

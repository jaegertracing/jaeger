-- span attribute "model" of type Str, hoisted into the column "model_column"
(s.`model_column` IS NOT NULL AND s.`model_column` = ?) -- binds [x]
-- span attribute "model" of type Int
arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value) -- binds [model 1]
-- span attribute "shard" of type Int
arrayExists((key, value) -> key = ? AND value >= ?, s.int_attributes.key, s.int_attributes.value) -- binds [shard 0]
-- span attribute "ratio" of type Double
arrayExists((key, value) -> key = ? AND value < ?, s.double_attributes.key, s.double_attributes.value) -- binds [ratio 0.5]
-- span attribute "retry" of type Bool
arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value) -- binds [retry true]
-- resource attribute "model" of type Str
arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value) -- binds [model x]
-- resource attribute "model" of type Int
arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value) -- binds [model 1]
-- resource attribute "shard" of type Int, hoisted into the column "shard_column"
(s.`shard_column` IS NOT NULL AND s.`shard_column` >= ?) -- binds [0]
-- resource attribute "ratio" of type Double
arrayExists((key, value) -> key = ? AND value < ?, s.resource_double_attributes.key, s.resource_double_attributes.value) -- binds [ratio 0.5]
-- resource attribute "retry" of type Bool
arrayExists((key, value) -> key = ? AND value = ?, s.resource_bool_attributes.key, s.resource_bool_attributes.value) -- binds [retry true]
-- scope attribute "model" of type Str
arrayExists((key, value) -> key = ? AND value = ?, s.scope_str_attributes.key, s.scope_str_attributes.value) -- binds [model x]
-- scope attribute "model" of type Int
arrayExists((key, value) -> key = ? AND value = ?, s.scope_int_attributes.key, s.scope_int_attributes.value) -- binds [model 1]
-- scope attribute "shard" of type Int
arrayExists((key, value) -> key = ? AND value >= ?, s.scope_int_attributes.key, s.scope_int_attributes.value) -- binds [shard 0]
-- scope attribute "ratio" of type Double
arrayExists((key, value) -> key = ? AND value < ?, s.scope_double_attributes.key, s.scope_double_attributes.value) -- binds [ratio 0.5]
-- scope attribute "retry" of type Bool
arrayExists((key, value) -> key = ? AND value = ?, s.scope_bool_attributes.key, s.scope_bool_attributes.value) -- binds [retry true]
-- event attribute "model" of type Str
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.events) -- binds [model x]
-- event attribute "model" of type Int
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.int_attributes.key, x.int_attributes.value), s.events) -- binds [model 1]
-- event attribute "shard" of type Int
arrayExists(x -> arrayExists((key, value) -> key = ? AND value >= ?, x.int_attributes.key, x.int_attributes.value), s.events) -- binds [shard 0]
-- event attribute "ratio" of type Double
arrayExists(x -> arrayExists((key, value) -> key = ? AND value < ?, x.double_attributes.key, x.double_attributes.value), s.events) -- binds [ratio 0.5]
-- event attribute "retry" of type Bool
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.bool_attributes.key, x.bool_attributes.value), s.events) -- binds [retry true]
-- link attribute "model" of type Str
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links) -- binds [model x]
-- link attribute "model" of type Int
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.int_attributes.key, x.int_attributes.value), s.links) -- binds [model 1]
-- link attribute "shard" of type Int
arrayExists(x -> arrayExists((key, value) -> key = ? AND value >= ?, x.int_attributes.key, x.int_attributes.value), s.links) -- binds [shard 0]
-- link attribute "ratio" of type Double
arrayExists(x -> arrayExists((key, value) -> key = ? AND value < ?, x.double_attributes.key, x.double_attributes.value), s.links) -- binds [ratio 0.5]
-- link attribute "retry" of type Bool
arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.bool_attributes.key, x.bool_attributes.value), s.links) -- binds [retry true]

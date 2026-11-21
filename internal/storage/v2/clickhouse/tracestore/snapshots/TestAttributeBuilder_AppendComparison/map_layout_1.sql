-- span attribute "model" of type Str, hoisted into the column "model_column"
-- binds [x]
(s.`model_column` IS NOT NULL AND s.`model_column` = ?)
-- span attribute "model" of type Int
-- binds [model model 1]
(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?)
-- span attribute "shard" of type Int
-- binds [shard shard 0]
(mapContains(s.int_attributes, ?) AND s.int_attributes[?] >= ?)
-- span attribute "ratio" of type Double
-- binds [ratio ratio 0.5]
(mapContains(s.double_attributes, ?) AND s.double_attributes[?] < ?)
-- span attribute "retry" of type Bool
-- binds [retry retry true]
(mapContains(s.bool_attributes, ?) AND s.bool_attributes[?] = ?)
-- resource attribute "model" of type Str
-- binds [model model x]
(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?)
-- resource attribute "model" of type Int
-- binds [model model 1]
(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?)
-- resource attribute "shard" of type Int, hoisted into the column "shard_column"
-- binds [0]
(s.`shard_column` IS NOT NULL AND s.`shard_column` >= ?)
-- resource attribute "ratio" of type Double
-- binds [ratio ratio 0.5]
(mapContains(s.resource_double_attributes, ?) AND s.resource_double_attributes[?] < ?)
-- resource attribute "retry" of type Bool
-- binds [retry retry true]
(mapContains(s.resource_bool_attributes, ?) AND s.resource_bool_attributes[?] = ?)
-- scope attribute "model" of type Str
-- binds [model model x]
(mapContains(s.scope_str_attributes, ?) AND s.scope_str_attributes[?] = ?)
-- scope attribute "model" of type Int
-- binds [model model 1]
(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] = ?)
-- scope attribute "shard" of type Int
-- binds [shard shard 0]
(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] >= ?)
-- scope attribute "ratio" of type Double
-- binds [ratio ratio 0.5]
(mapContains(s.scope_double_attributes, ?) AND s.scope_double_attributes[?] < ?)
-- scope attribute "retry" of type Bool
-- binds [retry retry true]
(mapContains(s.scope_bool_attributes, ?) AND s.scope_bool_attributes[?] = ?)
-- event attribute "model" of type Str
-- binds [model model x]
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.events)
-- event attribute "model" of type Int
-- binds [model model 1]
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.events)
-- event attribute "shard" of type Int
-- binds [shard shard 0]
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] >= ?, s.events)
-- event attribute "ratio" of type Double
-- binds [ratio ratio 0.5]
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] < ?, s.events)
-- event attribute "retry" of type Bool
-- binds [retry retry true]
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.events)
-- link attribute "model" of type Str
-- binds [model model x]
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links)
-- link attribute "model" of type Int
-- binds [model model 1]
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.links)
-- link attribute "shard" of type Int
-- binds [shard shard 0]
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] >= ?, s.links)
-- link attribute "ratio" of type Double
-- binds [ratio ratio 0.5]
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] < ?, s.links)
-- link attribute "retry" of type Bool
-- binds [retry retry true]
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.links)

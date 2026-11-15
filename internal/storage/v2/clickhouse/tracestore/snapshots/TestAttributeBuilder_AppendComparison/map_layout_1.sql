-- span attribute "model" of type Str
(s.`model_column` IS NOT NULL AND s.`model_column` = ?) -- binds [x]
-- span attribute "model" of type Int
(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?) -- binds [model model 1]
-- span attribute "shard" of type Int
(mapContains(s.int_attributes, ?) AND s.int_attributes[?] >= ?) -- binds [shard shard 0]
-- span attribute "ratio" of type Double
(mapContains(s.double_attributes, ?) AND s.double_attributes[?] < ?) -- binds [ratio ratio 0.5]
-- span attribute "retry" of type Bool
(mapContains(s.bool_attributes, ?) AND s.bool_attributes[?] = ?) -- binds [retry retry true]
-- resource attribute "model" of type Str
(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?) -- binds [model model x]
-- resource attribute "model" of type Int
(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?) -- binds [model model 1]
-- resource attribute "shard" of type Int
(s.`shard_column` IS NOT NULL AND s.`shard_column` >= ?) -- binds [0]
-- resource attribute "ratio" of type Double
(mapContains(s.resource_double_attributes, ?) AND s.resource_double_attributes[?] < ?) -- binds [ratio ratio 0.5]
-- resource attribute "retry" of type Bool
(mapContains(s.resource_bool_attributes, ?) AND s.resource_bool_attributes[?] = ?) -- binds [retry retry true]
-- scope attribute "model" of type Str
(mapContains(s.scope_str_attributes, ?) AND s.scope_str_attributes[?] = ?) -- binds [model model x]
-- scope attribute "model" of type Int
(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] = ?) -- binds [model model 1]
-- scope attribute "shard" of type Int
(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] >= ?) -- binds [shard shard 0]
-- scope attribute "ratio" of type Double
(mapContains(s.scope_double_attributes, ?) AND s.scope_double_attributes[?] < ?) -- binds [ratio ratio 0.5]
-- scope attribute "retry" of type Bool
(mapContains(s.scope_bool_attributes, ?) AND s.scope_bool_attributes[?] = ?) -- binds [retry retry true]
-- event attribute "model" of type Str
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.events) -- binds [model model x]
-- event attribute "model" of type Int
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.events) -- binds [model model 1]
-- event attribute "shard" of type Int
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] >= ?, s.events) -- binds [shard shard 0]
-- event attribute "ratio" of type Double
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] < ?, s.events) -- binds [ratio ratio 0.5]
-- event attribute "retry" of type Bool
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.events) -- binds [retry retry true]
-- link attribute "model" of type Str
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links) -- binds [model model x]
-- link attribute "model" of type Int
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.links) -- binds [model model 1]
-- link attribute "shard" of type Int
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] >= ?, s.links) -- binds [shard shard 0]
-- link attribute "ratio" of type Double
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] < ?, s.links) -- binds [ratio ratio 0.5]
-- link attribute "retry" of type Bool
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.links) -- binds [retry retry true]

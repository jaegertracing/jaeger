-- span attribute "model"
(s.`model_column` IS NOT NULL AND s.`model_column` = ?) -- binds [x]
-- span attribute "shard"
(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?) -- binds [shard shard 0]
-- span attribute "ratio"
(mapContains(s.double_attributes, ?) AND s.double_attributes[?] = ?) -- binds [ratio ratio 0.5]
-- span attribute "retry"
(mapContains(s.bool_attributes, ?) AND s.bool_attributes[?] = ?) -- binds [retry retry true]
-- resource attribute "model"
(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?) -- binds [model model x]
-- resource attribute "shard"
(s.`shard_column` IS NOT NULL AND s.`shard_column` = ?) -- binds [0]
-- resource attribute "ratio"
(mapContains(s.resource_double_attributes, ?) AND s.resource_double_attributes[?] = ?) -- binds [ratio ratio 0.5]
-- resource attribute "retry"
(mapContains(s.resource_bool_attributes, ?) AND s.resource_bool_attributes[?] = ?) -- binds [retry retry true]
-- scope attribute "model"
(mapContains(s.scope_str_attributes, ?) AND s.scope_str_attributes[?] = ?) -- binds [model model x]
-- scope attribute "shard"
(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] = ?) -- binds [shard shard 0]
-- scope attribute "ratio"
(mapContains(s.scope_double_attributes, ?) AND s.scope_double_attributes[?] = ?) -- binds [ratio ratio 0.5]
-- scope attribute "retry"
(mapContains(s.scope_bool_attributes, ?) AND s.scope_bool_attributes[?] = ?) -- binds [retry retry true]
-- event attribute "model"
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.events) -- binds [model model x]
-- event attribute "shard"
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.events) -- binds [shard shard 0]
-- event attribute "ratio"
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] = ?, s.events) -- binds [ratio ratio 0.5]
-- event attribute "retry"
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.events) -- binds [retry retry true]
-- link attribute "model"
arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links) -- binds [model model x]
-- link attribute "shard"
arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] = ?, s.links) -- binds [shard shard 0]
-- link attribute "ratio"
arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] = ?, s.links) -- binds [ratio ratio 0.5]
-- link attribute "retry"
arrayExists(x -> mapContains(x.bool_attributes, ?) AND x.bool_attributes[?] = ?, s.links) -- binds [retry retry true]

-- span attribute "model" of type Bool
-- binds [model]
(mapContains(s.bool_attributes, ?))
-- span attribute "model" of type Double
-- binds [model]
(mapContains(s.double_attributes, ?))
-- span attribute "model" of type Int
-- binds [model]
(mapContains(s.int_attributes, ?))
-- span attribute "model" of type Str
-- binds [model]
(mapContains(s.str_attributes, ?))
-- span attribute "model" of type Bytes
-- binds [@bytes@model]
(mapContains(s.complex_attributes, ?))
-- span attribute "model" of type Map
-- binds [@map@model]
(mapContains(s.complex_attributes, ?))
-- span attribute "model" of type Slice
-- binds [@slice@model]
(mapContains(s.complex_attributes, ?))
-- resource attribute "model" of type Bool
-- binds [model]
(mapContains(s.resource_bool_attributes, ?))
-- resource attribute "model" of type Double
-- binds [model]
(mapContains(s.resource_double_attributes, ?))
-- resource attribute "model" of type Int
-- binds [model]
(mapContains(s.resource_int_attributes, ?))
-- resource attribute "model" of type Str
-- binds [model]
(mapContains(s.resource_str_attributes, ?))
-- resource attribute "model" of type Bytes
-- binds [@bytes@model]
(mapContains(s.resource_complex_attributes, ?))
-- resource attribute "model" of type Map
-- binds [@map@model]
(mapContains(s.resource_complex_attributes, ?))
-- resource attribute "model" of type Slice
-- binds [@slice@model]
(mapContains(s.resource_complex_attributes, ?))
-- scope attribute "model" of type Bool
-- binds [model]
(mapContains(s.scope_bool_attributes, ?))
-- scope attribute "model" of type Double
-- binds [model]
(mapContains(s.scope_double_attributes, ?))
-- scope attribute "model" of type Int
-- binds [model]
(mapContains(s.scope_int_attributes, ?))
-- scope attribute "model" of type Str
-- binds [model]
(mapContains(s.scope_str_attributes, ?))
-- scope attribute "model" of type Bytes
-- binds [@bytes@model]
(mapContains(s.scope_complex_attributes, ?))
-- scope attribute "model" of type Map
-- binds [@map@model]
(mapContains(s.scope_complex_attributes, ?))
-- scope attribute "model" of type Slice
-- binds [@slice@model]
(mapContains(s.scope_complex_attributes, ?))
-- event attribute "model" of type Bool
-- binds [model]
arrayExists(x -> mapContains(x.bool_attributes, ?), s.events)
-- event attribute "model" of type Double
-- binds [model]
arrayExists(x -> mapContains(x.double_attributes, ?), s.events)
-- event attribute "model" of type Int
-- binds [model]
arrayExists(x -> mapContains(x.int_attributes, ?), s.events)
-- event attribute "model" of type Str
-- binds [model]
arrayExists(x -> mapContains(x.str_attributes, ?), s.events)
-- event attribute "model" of type Bytes
-- binds [@bytes@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.events)
-- event attribute "model" of type Map
-- binds [@map@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.events)
-- event attribute "model" of type Slice
-- binds [@slice@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.events)
-- link attribute "model" of type Bool
-- binds [model]
arrayExists(x -> mapContains(x.bool_attributes, ?), s.links)
-- link attribute "model" of type Double
-- binds [model]
arrayExists(x -> mapContains(x.double_attributes, ?), s.links)
-- link attribute "model" of type Int
-- binds [model]
arrayExists(x -> mapContains(x.int_attributes, ?), s.links)
-- link attribute "model" of type Str
-- binds [model]
arrayExists(x -> mapContains(x.str_attributes, ?), s.links)
-- link attribute "model" of type Bytes
-- binds [@bytes@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.links)
-- link attribute "model" of type Map
-- binds [@map@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.links)
-- link attribute "model" of type Slice
-- binds [@slice@model]
arrayExists(x -> mapContains(x.complex_attributes, ?), s.links)

-- span attribute "model" of type Bool
-- binds [model]
(has(s.bool_attributes.key, ?))
-- span attribute "model" of type Double
-- binds [model]
(has(s.double_attributes.key, ?))
-- span attribute "model" of type Int
-- binds [model]
(has(s.int_attributes.key, ?))
-- span attribute "model" of type Str
-- binds [model]
(has(s.str_attributes.key, ?))
-- span attribute "model" of type Bytes
-- binds [@bytes@model]
(has(s.complex_attributes.key, ?))
-- span attribute "model" of type Map
-- binds [@map@model]
(has(s.complex_attributes.key, ?))
-- span attribute "model" of type Slice
-- binds [@slice@model]
(has(s.complex_attributes.key, ?))
-- resource attribute "model" of type Bool
-- binds [model]
(has(s.resource_bool_attributes.key, ?))
-- resource attribute "model" of type Double
-- binds [model]
(has(s.resource_double_attributes.key, ?))
-- resource attribute "model" of type Int
-- binds [model]
(has(s.resource_int_attributes.key, ?))
-- resource attribute "model" of type Str
-- binds [model]
(has(s.resource_str_attributes.key, ?))
-- resource attribute "model" of type Bytes
-- binds [@bytes@model]
(has(s.resource_complex_attributes.key, ?))
-- resource attribute "model" of type Map
-- binds [@map@model]
(has(s.resource_complex_attributes.key, ?))
-- resource attribute "model" of type Slice
-- binds [@slice@model]
(has(s.resource_complex_attributes.key, ?))
-- scope attribute "model" of type Bool
-- binds [model]
(has(s.scope_bool_attributes.key, ?))
-- scope attribute "model" of type Double
-- binds [model]
(has(s.scope_double_attributes.key, ?))
-- scope attribute "model" of type Int
-- binds [model]
(has(s.scope_int_attributes.key, ?))
-- scope attribute "model" of type Str
-- binds [model]
(has(s.scope_str_attributes.key, ?))
-- scope attribute "model" of type Bytes
-- binds [@bytes@model]
(has(s.scope_complex_attributes.key, ?))
-- scope attribute "model" of type Map
-- binds [@map@model]
(has(s.scope_complex_attributes.key, ?))
-- scope attribute "model" of type Slice
-- binds [@slice@model]
(has(s.scope_complex_attributes.key, ?))
-- event attribute "model" of type Bool
-- binds [model]
arrayExists(x -> has(x.bool_attributes.key, ?), s.events)
-- event attribute "model" of type Double
-- binds [model]
arrayExists(x -> has(x.double_attributes.key, ?), s.events)
-- event attribute "model" of type Int
-- binds [model]
arrayExists(x -> has(x.int_attributes.key, ?), s.events)
-- event attribute "model" of type Str
-- binds [model]
arrayExists(x -> has(x.str_attributes.key, ?), s.events)
-- event attribute "model" of type Bytes
-- binds [@bytes@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.events)
-- event attribute "model" of type Map
-- binds [@map@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.events)
-- event attribute "model" of type Slice
-- binds [@slice@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.events)
-- link attribute "model" of type Bool
-- binds [model]
arrayExists(x -> has(x.bool_attributes.key, ?), s.links)
-- link attribute "model" of type Double
-- binds [model]
arrayExists(x -> has(x.double_attributes.key, ?), s.links)
-- link attribute "model" of type Int
-- binds [model]
arrayExists(x -> has(x.int_attributes.key, ?), s.links)
-- link attribute "model" of type Str
-- binds [model]
arrayExists(x -> has(x.str_attributes.key, ?), s.links)
-- link attribute "model" of type Bytes
-- binds [@bytes@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.links)
-- link attribute "model" of type Map
-- binds [@map@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.links)
-- link attribute "model" of type Slice
-- binds [@slice@model]
arrayExists(x -> has(x.complex_attributes.key, ?), s.links)

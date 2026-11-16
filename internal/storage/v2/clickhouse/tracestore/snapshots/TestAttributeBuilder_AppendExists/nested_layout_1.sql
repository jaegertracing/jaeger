-- span attribute "model"
(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- resource attribute "model"
(has(s.resource_bool_attributes.key, ?) OR has(s.resource_double_attributes.key, ?) OR has(s.resource_int_attributes.key, ?) OR has(s.resource_str_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- scope attribute "model"
(has(s.scope_bool_attributes.key, ?) OR has(s.scope_double_attributes.key, ?) OR has(s.scope_int_attributes.key, ?) OR has(s.scope_str_attributes.key, ?) OR has(s.scope_complex_attributes.key, ?) OR has(s.scope_complex_attributes.key, ?) OR has(s.scope_complex_attributes.key, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- event attribute "model"
arrayExists(x -> has(x.bool_attributes.key, ?) OR has(x.double_attributes.key, ?) OR has(x.int_attributes.key, ?) OR has(x.str_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?), s.events) -- binds [model model model model @bytes@model @map@model @slice@model]
-- link attribute "model"
arrayExists(x -> has(x.bool_attributes.key, ?) OR has(x.double_attributes.key, ?) OR has(x.int_attributes.key, ?) OR has(x.str_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?), s.links) -- binds [model model model model @bytes@model @map@model @slice@model]

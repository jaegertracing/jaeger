-- span attribute "model"
(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- resource attribute "model"
(mapContains(s.resource_bool_attributes, ?) OR mapContains(s.resource_double_attributes, ?) OR mapContains(s.resource_int_attributes, ?) OR mapContains(s.resource_str_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- scope attribute "model"
(mapContains(s.scope_bool_attributes, ?) OR mapContains(s.scope_double_attributes, ?) OR mapContains(s.scope_int_attributes, ?) OR mapContains(s.scope_str_attributes, ?) OR mapContains(s.scope_complex_attributes, ?) OR mapContains(s.scope_complex_attributes, ?) OR mapContains(s.scope_complex_attributes, ?)) -- binds [model model model model @bytes@model @map@model @slice@model]
-- event attribute "model"
arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.events) -- binds [model model model model @bytes@model @map@model @slice@model]
-- link attribute "model"
arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.links) -- binds [model model model model @bytes@model @map@model @slice@model]

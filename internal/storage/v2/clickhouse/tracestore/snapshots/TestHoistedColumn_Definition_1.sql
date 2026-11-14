`span_str` Nullable(String) MATERIALIZED if(has(str_attributes.key, 'gen_ai.request.model'), str_attributes.value[indexOf(str_attributes.key, 'gen_ai.request.model')], NULL) CODEC(ZSTD(1)),
`span_int` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1)),
`span_double` Nullable(Float64) MATERIALIZED if(has(double_attributes.key, 'ratio'), double_attributes.value[indexOf(double_attributes.key, 'ratio')], NULL) CODEC(ZSTD(1)),
`span_bool` Nullable(Bool) MATERIALIZED if(has(bool_attributes.key, 'cached'), bool_attributes.value[indexOf(bool_attributes.key, 'cached')], NULL) CODEC(ZSTD(1)),
`resource_int` Nullable(Int64) MATERIALIZED if(has(resource_int_attributes.key, 'shard'), resource_int_attributes.value[indexOf(resource_int_attributes.key, 'shard')], NULL) CODEC(ZSTD(3)),
`index` Nullable(String) MATERIALIZED if(has(resource_str_attributes.key, 'it\'s a \\key'), resource_str_attributes.value[indexOf(resource_str_attributes.key, 'it\'s a \\key')], NULL) CODEC(ZSTD(1)),
`map_span_int` Nullable(Int64) MATERIALIZED if(mapContains(int_attributes, 'gen_ai.usage.input_tokens'), int_attributes['gen_ai.usage.input_tokens'], NULL) CODEC(ZSTD(1)),
`map_resource_str` Nullable(String) MATERIALIZED if(mapContains(resource_str_attributes, 'it\'s a \\key'), resource_str_attributes['it\'s a \\key'], NULL) CODEC(ZSTD(1))

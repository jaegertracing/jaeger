-- non-Distributed table, column matches
no error

-- non-Distributed table, column is missing
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- non-Distributed table, another type
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(String) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- non-Distributed table, an ordinary column
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) CODEC(ZSTD(1))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- non-Distributed table, a default in place of a materialized column
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) DEFAULT if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- non-Distributed table, another expression
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'another.key'), int_attributes.value[indexOf(int_attributes.key, 'another.key')], NULL) CODEC(ZSTD(1))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- non-Distributed table, another codec
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(3))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- non-Distributed table beside an unrelated spans_local, column is missing
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- Distributed table, both tables match
no error

-- Distributed table, only the Distributed table has the column
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans_local ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- Distributed table, only the storage table has the column
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- Distributed table, neither table has the column
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans_local ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1)); ALTER TABLE spans ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- Distributed table, the storage table declares another codec
hoisted attribute column "input_tokens" does not match its configuration: the spans_local table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(3))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- Distributed table, both tables declare another codec
hoisted attribute column "input_tokens" does not match its configuration: the spans_local table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(3))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(3))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- Distributed table, the storage table lacks the column and the Distributed table declares another codec
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans_local ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))
hoisted attribute column "input_tokens" does not match its configuration: the spans table declares it as [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(3))], and the configuration needs [Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))]; a changed hoisted attribute needs a schema migration

-- Distributed table without a spans_local table, column is missing
hoisted attribute column "input_tokens" is missing; add it with: ALTER TABLE spans ADD COLUMN `input_tokens` Nullable(Int64) MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'), int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')], NULL) CODEC(ZSTD(1))

-- spans_local without spans
hoisted attributes are configured, but the spans table does not exist; create the schema first

-- no tables
hoisted attributes are configured, but the spans table does not exist; create the schema first

-- table lookup fails
failed to look up the spans table: assert.AnError general error for testing

-- table lookup scan fails
failed to look up the spans table: assert.AnError general error for testing

-- table lookup rows fail
failed to look up the spans table: assert.AnError general error for testing

-- column lookup fails
failed to read the hoisted attribute column "input_tokens" of the spans table: assert.AnError general error for testing

-- column lookup scan fails
failed to read the hoisted attribute column "input_tokens" of the spans table: assert.AnError general error for testing

-- column lookup rows fail
failed to read the hoisted attribute column "input_tokens" of the spans table: assert.AnError general error for testing

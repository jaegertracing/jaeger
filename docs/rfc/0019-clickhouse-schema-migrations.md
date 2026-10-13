# RFC 0019: ClickHouse Schema Migrations

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-10-13
- **Last Updated:** 2026-10-13
- **Issue:** [#9789](https://github.com/jaegertracing/jaeger/issues/9789)
- **Related:** [ADR-008 (ClickHouse storage schema)](../adr/008-clickhouse-storage-schema.md) · [RFC 0017 (schema tuning adopted from ClickStack)](./0017-clickhouse-schema-vs-clickstack.md) · [#9796 (golang-migrate PR)](https://github.com/jaegertracing/jaeger/pull/9796) · [#9799 (OTLP span columns PR)](https://github.com/jaegertracing/jaeger/pull/9799) · [#9813 (configurable table engine)](https://github.com/jaegertracing/jaeger/pull/9813)

---

## Abstract

The ClickHouse backend creates its twelve tables and materialized views at startup from one `CREATE ... IF NOT EXISTS` template per object, and has no way to change an object that already exists. The first change to the schema since it reached users, the missing OTLP span columns of #9789, therefore has to establish how the schema is amended from now on. This RFC describes the mechanism as it stands, states what a replacement has to satisfy, and analyses four families of approach: versioned delta files applied by a migration framework, as #9789 sketched and #9796 implements with golang-migrate; the same deltas applied by a small in-house migrator; a full copy of the schema per version, which is how the Cassandra backend works; and reconciliation, where the templates stay the single declarative description of the desired schema and the backend computes at startup the `ALTER` statements that bring the live database to it, in the manner of Kubernetes controllers and Terraform.

Reconciliation is the recommendation. It keeps today's per-object templates as the living source of truth, needs no version numbering, frozen files or golden dumps, and gives operators who manage the schema themselves a printed plan of exactly what their database lacks. ClickHouse makes the diff tractable without a SQL parser: the desired objects are created in a scratch database and compared with the live ones through the system catalogs, so both sides are normalized by the same server. At startup the backend applies only the additive part of the plan, new objects, columns and indexes, which is idempotent and commutes across concurrent instances and releases; every other change, a codec, default, setting, TTL or view query, is reported and applied by a one-shot operator command, because ClickHouse offers no primitive that can serialize such changes across instances safely. The engine refuses every change that would rewrite data and never drops anything. A survey of third-party reconcilers for ClickHouse found none that is both embeddable and under a free license, so the diff engine is written in-house, informed by the designs of those tools. Release-time snapshots and numbered migration files are produced from the same engine for people who want a versioned audit trail.

---

## 1. Motivation

Three things force the question now.

1. **A schema change is pending.** #9789 adds ten columns to the `spans` table. Against an existing deployment the startup statements are no-ops, so without a migration mechanism the columns never appear and the writer fails on its first insert after the upgrade.
2. **RFC 0017 queues more.** The codecs, table settings and skip indexes that RFC 0017 adopts are each an `ALTER TABLE` that can run at startup, and its `LowCardinality` wrappers are a column type change that rewrites data and needs an operator procedure (§2.3). The backend is going to change its schema several times in the next few releases, and each change needs a defined path.
3. **Two PRs already implement one answer.** #9796 and #9799 adopt golang-migrate along the lines sketched in #9789 and put the whole baseline schema in a single `000001_initial_schema.up.sql`. Review of those PRs showed that the file layout, the test design and the library's fit need deciding before the code, which is what an RFC is for.

The Cassandra backend is the cautionary example. Its schema lives in `v001.cql.tmpl` through `v004.cql.tmpl` under [`internal/storage/v1/cassandra/schema/`](../../internal/storage/v1/cassandra/schema/), each a complete copy of every table, and upgrades are operator-run shell scripts in a `migration/` directory that the binary knows nothing about. A change to one table meant copying the entire schema into a new file, and nothing recorded which version a running keyspace was at. Whatever ClickHouse adopts has to do better on both counts.

---

## 2. Current State

### 2.1 Files

The schema is twelve files under [`internal/storage/v2/clickhouse/sql/`](../../internal/storage/v2/clickhouse/sql/), one per object, each embedded as a string variable in `queries.go`:

| File | Object | Templated |
| --- | --- | --- |
| `create_spans_table.sql` | `spans` table | yes |
| `create_services_table.sql` | `services` table | yes |
| `create_services_mv.sql` | `services_mv` view | no |
| `create_operations_table.sql` | `operations` table | yes |
| `create_operations_mv.sql` | `operations_mv` view | no |
| `create_trace_id_timestamps_table.sql` | `trace_id_timestamps` table | yes |
| `create_trace_id_timestamps_mv.sql` | `trace_id_timestamps_mv` view | no |
| `create_attribute_metadata_table.sql` | `attribute_metadata` table | yes |
| `create_attribute_metadata_mv.sql` | `attribute_metadata_mv` view | no |
| `create_event_attribute_metadata_mv.sql` | `event_attribute_metadata_mv` view | no |
| `create_link_attribute_metadata_mv.sql` | `link_attribute_metadata_mv` view | no |
| `create_dependencies_table.sql` | `dependencies` table | yes |

The six table files are `text/template` documents rendered with `schemaTemplateParams` in `factory.go`:

```go
type schemaTemplateParams struct {
    TTLSeconds           int64  // zero renders no TTL clause
    MergeTree            string // ENGINE clause body for raw-row tables
    AggregatingMergeTree string // ENGINE clause body for derived tables
}
```

`TTLSeconds` comes from the `ttl` setting and renders `TTL start_time + INTERVAL n SECOND DELETE` on `spans` and `TTL end + ...` on `trace_id_timestamps`. The two engine strings come from the `table_engine` block that #9813 introduced: `merge_tree: {}` renders `MergeTree` and `AggregatingMergeTree`, and `replicated: {...}` renders `ReplicatedMergeTree('/path', 'replica')` and its aggregating counterpart. The materialized views are plain SQL because each writes `TO` its target table and carries no engine of its own.

### 2.2 Startup behavior

`newSchemaBuilder` holds the twelve statements in dependency order, tables before the views that read them, and renders the six templates. With `create_schema: true`, `NewFactory` executes the statements one by one over the native connection before serving traffic. With `create_schema: false`, nothing runs and the operator owns the tables.

Nothing is recorded about what ran. There is no version table, no fingerprint of the DDL, and no check at startup that the tables match what the binary expects. A mismatch surfaces as an insert or query error on the first request that touches the missing column.

### 2.3 What ClickHouse allows after `CREATE`

The shape of any migration mechanism follows from which parts of a table can change in place.

| Change | Statement | Cost | Replicated tables |
| --- | --- | --- | --- |
| Add a column | `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` | metadata only¹ | propagates through Keeper |
| Add a skip index | `ALTER TABLE ... ADD INDEX IF NOT EXISTS` | metadata only; existing parts are indexed on merge or by `MATERIALIZE INDEX` | propagates |
| Change a column's codec | `ALTER TABLE ... MODIFY COLUMN ... CODEC(...)` | metadata only; parts are re-encoded on merge | propagates |
| Replace a skip index | `DROP INDEX` then `ADD INDEX` | `DROP INDEX` is a mutation that only deletes index files; the new index is built on merge or by `MATERIALIZE INDEX` | propagates |
| Change a column's type, including adding `LowCardinality` | `ALTER TABLE ... MODIFY COLUMN ... <type>` | a mutation that rewrites every part² | propagates |
| Change a table setting | `ALTER TABLE ... MODIFY SETTING` | metadata only | replica-local³ |
| Change or remove TTL | `ALTER TABLE ... MODIFY TTL`, `ALTER TABLE ... REMOVE TTL` | a mutation by default; metadata only with `materialize_ttl_after_modify = 0`, after which the new expression applies as parts merge⁴ | propagates |
| Change a view's query | `ALTER TABLE <view> MODIFY QUERY` | metadata only; generally available since ClickHouse 23.11 | replica-local³ |
| Extend the sorting key | `ALTER TABLE ... MODIFY ORDER BY` | metadata only, and only by appending columns added in the same statement | propagates |
| Drop a column | `ALTER TABLE ... DROP COLUMN` | destroys data | propagates |
| Change `ENGINE`, `PARTITION BY`, or the existing sorting key | not possible | requires a new table and a copy | n/a |

- ¹ The column is added to each part's metadata; existing parts read the column's default until they are merged.
- ² A mutation is background work over every affected part, which for a large `spans` table is hours of I/O. It is the reason type changes are operator procedures rather than startup steps (§5.4).
- ³ A `MODIFY SETTING` on a `Replicated*` table changes only the replica that received it, which the server's own code states (`StorageReplicatedMergeTree::alter` does not replicate a settings-only alter) and the documentation does not; a materialized view has engine `MaterializedView`, not a `Replicated*` engine, so its `MODIFY QUERY` is local too. Reaching every replica needs the fan-out of §2.4.
- ⁴ With materialization off, existing parts keep their rows until a merge re-evaluates the TTL, which is the behavior a day-partitioned table with `ttl_only_drop_parts` wants anyway.

Replication covers less than it seems. A column, index or codec `ALTER` on a `Replicated*` table is recorded in Keeper and applied on every replica, so a statement issued through one connection reaches the whole shard. A `CREATE TABLE`, a `MODIFY SETTING` and a view's `MODIFY QUERY` are local to the server the client reached, and reaching the others needs `ON CLUSTER`, a database with the `Replicated` database engine, or a connection to each. §5.6 draws the consequence.

### 2.4 Facilities ClickHouse offers a schema tool

- **System catalogs.** `system.tables` has `engine_full`, `sorting_key`, `partition_key`, `create_table_query` and, for views, `as_select`; `system.columns` has each column's `type`, `default_kind`, `default_expression`, `compression_codec` and `position`, with `Nested` groups flattened to `events.name Array(String)`; `system.data_skipping_indices` has each index's `type_full`, `expr` and `granularity`. Everything a diff needs is a query away, normalized by the server.
- **Scratch databases.** `CREATE DATABASE` is cheap, so the desired DDL can be created under a throwaway database and compared with the live objects through the catalogs above, which avoids writing a ClickHouse DDL parser.
- **A cross-process lock.** ClickHouse has no advisory locks, but an `INSERT` into a `KeeperMap` table issued with the query setting `keeper_map_strict_mode = 1` fails when the key exists, which is a compare-and-set with Keeper as the arbiter. It needs a 2023 or later ClickHouse release, a Keeper ensemble, and the opt-in `keeper_map_path_prefix` server setting, which no default configuration sets; a server without it refuses to create the table with a recognizable error.
- **DDL fan-out.** The `Replicated` database engine replays every DDL statement issued to one replica on all of them, and is the default on ClickHouse Cloud; `ON CLUSTER <name>` does the same per statement on a self-hosted cluster with an ordinary `Atomic` database.

### 2.5 How the other backends do it

Cassandra is covered in §1. Elasticsearch and OpenSearch have no comparable problem: their mapping is an index template that the binary installs at startup, and because indices roll, by day or by alias rollover, a new mapping applies to the next index without altering the previous one. Badger and memory have no persistent schema.

---

## 3. Requirements

The mechanism is judged against these criteria, which also form the rows of the comparison in §4.6.

1. **Per-object deltas.** A change to one column of one table is a change to one short file, never a copy of the schema.
2. **One source of truth.** What a fresh install creates and what an upgraded install ends up with derive from one set of files, not two paths kept equal by discipline.
3. **Template parameters survive.** `ttl` and `table_engine` are properties of the deployment, not of the schema, and every `CREATE` has to render them. A replicated deployment that gets local tables because a migration file hard-codes `MergeTree` is a silent data-loss configuration.
4. **The database's state is knowable.** The binary can tell what the live schema lacks, and an operator can tell what a cluster has.
5. **Safe with concurrent instances.** Several Jaeger instances start together against one database. The result has to be correct whoever finishes second.
6. **Operator-managed schema stays possible.** With `create_schema: false` the operator applies the DDL by hand, and the binary gives them exactly what to apply.
7. **Readable current schema.** A developer changing the writer can find out what the `spans` table looks like today by reading a file, not by replaying history.
8. **Small dependency footprint.** The backend pulls in no database drivers or connection paths it does not otherwise use.
9. **Little bookkeeping imposed on contributors.** Version numbers, ordering rules, frozen files and generated golden files are each a thing a contributor has to learn and a reviewer has to check.
10. **What runs in production is reviewable.** A reviewer of a schema PR, and an operator before an upgrade, can see the statements that will execute.

---

## 4. Approaches

### 4.1 Versioned deltas applied by a migration framework

This is the model of Flyway, Liquibase, golang-migrate and goose, and the one #9789 sketched: a numbered sequence of forward-only files, a table recording which have run, and a library that applies the pending ones in order. The current schema is the sum of the files, so the baseline is version 1 and every change is a new file. #9796 implements it with golang-migrate.

Two properties of golang-migrate bear on the choice. The layout is free: the library reads each version as one `io.Reader` from any `fs.FS`, so per-object files can be rendered and concatenated in memory, and the single baseline file in #9796 is a choice rather than a constraint. The ClickHouse driver is a poor fit for this backend: it needs a second connection through `database/sql`, its process-local lock leaves concurrent instances to idempotence alone, and its version-table handling and its refusal to start against a newer recorded version each contradict a requirement of §3. The specifics were raised in the review of #9796 and are not repeated here; what remains of the library once they are worked around is the version table and the ordered walk, about a hundred lines.

### 4.2 Versioned deltas applied by an in-house migrator

The same layout, one directory per version holding one file per object the version changes, with the walk and the version table written in the backend over the native connection. The driver problems above disappear, and the cross-version interleaving of concurrent instances remains, needing either the `KeeperMap` lock of §2.4 or a rule that a version never reassigns a property an earlier version assigned.

The bookkeeping stays: version numbers, a frozen baseline that is no longer the readable schema, a golden `SHOW CREATE TABLE` dump to read instead, and a rule that a released version is never edited.

### 4.3 Full copy of the schema per version

The Cassandra model. Each version is a complete, readable schema, and the diff between two versions is whatever a human writes into a migration script. It fails requirement 1 by construction and records nothing in the database, and is listed because it is the precedent in this repository.

### 4.4 Living templates for fresh installs, deltas for upgrades

Keep today's templates as the readable current schema and use them for a fresh install, recording the newest version number without running the deltas; an existing database applies the deltas. This scores well on readability and loses on requirement 2: the templates and the deltas are two descriptions of the same schema, and only a parity test keeps them equal. A template edit with no matching delta passes review and breaks upgrades silently.

### 4.5 Reconciliation

Describe the desired schema once, declaratively, and let the tool compute the statements that bring the live database to it. This is how Kubernetes controllers treat resources, how Terraform treats infrastructure, and how Atlas, Skeema, sqldef and Prisma treat relational schemas. For ClickHouse specifically it is the model of Atlas's ClickHouse driver, PostHog's `chschema`, Noibu's `housekeeper` and the `diff` command of the Python `clickhouse-migrations`.

For this backend the desired schema is today's twelve templates, unchanged and still edited in place. At startup the backend renders them, creates each object under a scratch database, reads both the scratch and the live objects from the system catalogs, and diffs them per object. The diff vocabulary is the rows of §2.3 that do not rewrite row data: `ADD COLUMN`, `ADD INDEX`, `DROP INDEX` followed by `ADD INDEX` to replace one, `MODIFY COLUMN ... CODEC`, `MODIFY SETTING`, `MODIFY TTL`, `MODIFY QUERY`, and `CREATE` for an object that does not exist. A TTL difference is applied without materializing it, so it takes effect as parts merge. Anything else the diff detects, a type change, a different sorting key, partition key or engine family, is reported and refused, because each is a mutation or a rebuild. Anything present in the live table and absent from the desired schema is left alone, because drop versus rename is undecidable from a diff and either loses data. The plan is a list of statements; with `create_schema: true` the backend applies it, with `create_schema: false` it prints it and starts, and a command prints it on demand.

Two properties follow that no versioned design has. The templates are the single readable source of truth, with no numbering, no frozen files and no golden dump. And a database that drifted, because an operator added an index by hand or an earlier version of the binary created a table slightly differently, is reconciled toward the desired state rather than assumed to match a version number.

A versioned audit trail is a product of the same engine rather than its input: at release time, diffing the previous release's rendered schema against the current one through a scratch database yields a numbered migration file to commit, for people who want one.

#### Third-party reconcilers for ClickHouse

The engine, with its verification step, is several hundred lines, so a library was looked for first. None is both embeddable in a Go binary and under a free license.

| Tool | Desired-state input | Diff method | License | Embeddable |
| --- | --- | --- | --- | --- |
| Atlas | HCL or SQL | live introspection, plan | Apache 2.0 core; the ClickHouse driver is in the paid Pro tier | ❌ |
| PostHog/chschema | HCL | live introspection, `diff -sql`; flags engine, `ORDER BY` and `PARTITION BY` changes as unsafe | MIT | ❌ Go, but all code under `internal/`, CLI only |
| Noibu/housekeeper | SQL DDL file | generates `ALTER`s, `ON CLUSTER` | GPL-3.0 | ❌ license |
| anytoe/chsync | live or `.sql` snapshot | catalog diff | MIT | ❌ Go under `internal/`; no indexes, codecs or TTL; self-described as not battle-tested |
| zifter/clickhouse-migrations 0.14 | `schema.sql` | scratch database plus catalog diff; refuses engine, `ORDER BY`, `PARTITION BY`, `SETTINGS`; comments out drops unless `--allow-destructive`; `KeeperMap` lock | MIT | ❌ Python |
| Liquibase with the ClickHouse extension | changelog | none for ClickHouse (`diffChangeLog` is not supported by the extension) | Apache 2.0 | ❌ Java, imperative |

Two of these matter as prior art even though none is adopted. The Python tool diffs through a scratch database exactly as §5 proposes and refuses the same class of changes, which is independent evidence that the method works on ClickHouse, and it implements the `KeeperMap` lock of §2.4. `chschema`'s unsafe-change classification matches §2.3.

### 4.6 Comparison

| Criterion (§3) | 4.1 golang-migrate deltas | 4.2 in-house deltas | 4.3 full copies | 4.4 living templates + deltas | **4.5 reconciliation** |
| --- | --- | --- | --- | --- | --- |
| 1. Per-object deltas | 🟨¹ | 🟢 | ❌ | 🟢 | 🟢 |
| 2. One source of truth | 🟢 | 🟢 | 🟢 | 🟨² | 🟢 |
| 3. Template parameters survive | 🟢³ | 🟢 | 🟢 | 🟢 | 🟢 |
| 4. Database state knowable | 🟨⁴ | 🟨⁴ | ❌ | 🟨⁴ | 🟢⁵ |
| 5. Safe with concurrent instances | ❌⁶ | 🟨⁷ | n/a | 🟨⁷ | 🟢⁸ |
| 6. Operator-managed schema | 🟨⁹ | 🟨⁹ | 🟢 | 🟨⁹ | 🟢¹⁰ |
| 7. Readable current schema | 🟨¹¹ | 🟨¹¹ | 🟢 | 🟢 | 🟢 |
| 8. Dependency footprint | 🟨¹² | 🟢 | 🟢 | 🟢 | 🟢 |
| 9. Little bookkeeping | ❌ | ❌ | 🟨 | ❌ | 🟢¹³ |
| 10. Production statements reviewable | 🟢 | 🟢 | 🟨 | 🟢 | 🟨¹⁴ |
| Code to own | 🟢 glue only | 🟨 ~100 lines | 🟢 | 🟨 | ❌ several hundred lines |

- ¹ Deltas from version 2 on, but the baseline is one long file and the per-object organization is lost when the framework is adopted.
- ² Templates and deltas are two descriptions kept equal only by a parity test.
- ³ As #9796 stands it renders the baseline through the templates; the earlier shape its review commented on hard-coded the engine.
- ⁴ A version number, which says what was recorded, not what the tables contain; drift is invisible.
- ⁵ The plan is computed from the live catalog, so what is reported is what the database actually lacks.
- ⁶ The library's version table creation is not idempotent, so first starts race, and pending versions can interleave across instances.
- ⁷ Correct if every statement is idempotent and no version reassigns an earlier version's property; a `KeeperMap` lock would make it 🟢.
- ⁸ Startup applies only additive statements, which are idempotent and commute up to a difference that the next plan reports (§5.5), so no coordination is needed. Non-additive statements run only from a one-shot operator command.
- ⁹ The operator receives "versions 2 and 3 are pending" and the files, and is trusted to apply them whole.
- ¹⁰ The operator receives the exact statements their database lacks, computed against it.
- ¹¹ Through a generated golden dump rather than the authored files.
- ¹² A second `database/sql` connection for the library's driver.
- ¹³ No version table, no numbering and no frozen files; the database state is read, not recorded.
- ¹⁴ The PR shows the template edit, not the `ALTER`; the statements are visible in the release-time migration file (§5.7) and in the printed plan.

Reconciliation is the recommendation. The rows that decided it are 4, 6, 7 and 9: the plan is computed from what the database actually contains, the operator gets the exact statements they lack, the templates stay the readable schema, and contributors learn no numbering or freezing rules. The row it loses, code to own, is catalog comparison plus a verification step, a shape the prior art shows to be solved, and row 10 is recovered by the release-time migration files. The in-house migrator of §4.2 is the fallback if the diff engine proves harder than estimated; it shares the per-object layout and the idempotence rules, so the templates and tests carry over.

---

## 5. Design

### 5.1 Desired state

The twelve files stay where they are and keep their names. They remain the complete description of the desired schema, edited in place when the schema changes: the OTLP columns of #9789 are ten lines added to `create_spans_table.sql`, and RFC 0017's codecs are edits to the same file. The templates are rendered with `schemaTemplateParams` as today.

### 5.2 The reconciler

```go
// schemaReconciler computes and applies the statements that bring the live schema
// to the rendered templates.
type schemaReconciler struct {
    conn   driver.Conn
    params schemaTemplateParams
}

// plan renders the desired objects into a scratch database, reads both scratch and
// live objects from the system catalogs, and returns the statements that would make
// the live objects match, in template order so that a table is created or altered
// before the views that read it.
func (r *schemaReconciler) plan(ctx context.Context) (schemaPlan, error)

// apply executes the statements of a plan in order and fails without executing
// anything if the plan carries a refusal.
func (r *schemaReconciler) apply(ctx context.Context, p schemaPlan) error

type schemaPlan struct {
    Statements []string         // CREATEs and metadata-only ALTERs, in template order
    Refused    []schemaRefusal
}

type schemaRefusal struct {
    Object    string
    Reason    string // e.g. "column duration: type Int64 differs from UInt64"
    Statement string // what an operator would run, for the message
}
```

`plan` does the following, all through the native connection the factory already holds:

1. Create a scratch database `jaeger_schema_<random>` with an explicit `ENGINE = Atomic`, so that it stays on the connected server where the default database engine is `Replicated`; a managed service that refuses `Atomic` gets a replicated scratch database instead, which is dropped the same way and costs a few metadata round trips. The reconciler then opens a client of its own to that same single address, with one connection and with the scratch database as the connection's default database, and renders and creates every object through it. The dedicated client is needed because the factory's `driver.Conn` is a pool over every configured address, and a scratch database created through one transport is not visible through another server's. The default database is needed because the templates name tables without a qualifier, so a materialized view created through a connection whose default is the live database would bind its `TO` table and its `FROM` to the live tables, fail to create when it selects a column the live table does not yet have, and receive live inserts while it exists. Scratch tables are rendered with the local engine family regardless of `table_engine`, because a `Replicated*` scratch table would register a Keeper path; the live engine is checked against the configured family directly (step 3).
2. For each desired object, read `system.tables`, `system.columns` and `system.data_skipping_indices` for both the scratch and the live copy. ClickHouse qualifies every table reference with its database at `CREATE` time, so `as_select` and `create_table_query` are compared with the database qualifier stripped on both sides. A first-level `Nested` group is flattened to one `Array` column per member on both sides; a `Nested` group inside another, such as the attribute groups under `events` and `links`, stays one column of type `Array(Nested(...))`, so a change inside it is a type change.
3. Per object, compute the difference. A live object that is missing yields its rendered `CREATE` with the deployment's engine. A missing column yields `ADD COLUMN IF NOT EXISTS` placed `AFTER` its predecessor; a missing index yields `ADD INDEX IF NOT EXISTS`; an index whose `type_full`, `expr` or `granularity` differs yields `DROP INDEX IF EXISTS` followed by `ADD INDEX`; a codec difference yields `MODIFY COLUMN ... CODEC`; a setting difference yields `MODIFY SETTING`; a TTL difference yields `MODIFY TTL ... SETTINGS materialize_ttl_after_modify = 0`, or `REMOVE TTL` when the desired schema has none; a default expression difference yields `MODIFY COLUMN ... DEFAULT`; a view whose `as_select` differs yields `MODIFY QUERY` with the query text taken from the rendered template, not from the scratch view. The table under "What is compared" below is the complete list of properties compared and of what each difference yields; a difference with no statement is a refusal. Columns and indexes present live and absent from the desired schema yield nothing. After the additive statements have run, `plan` runs again and any additive statements it still finds are applied, up to three rounds, because two releases that both found a table missing both plan only its `CREATE`, and the one whose `CREATE` lost still has columns to add.
4. Drop the scratch database, also on error.

Idempotence is a property of the vocabulary: every applicable statement is a no-op when the live object already matches, so two instances applying the same plan both succeed, and a plan computed after `apply` is empty.

#### What is compared

The comparison is defined per object kind over the catalog columns below. A property not in this table is not compared, so an empty plan certifies exactly these properties and nothing else.

| Object | Property | Source | On difference |
| --- | --- | --- | --- |
| table | exists | `system.tables` | `CREATE` (additive) |
| table | engine family (`MergeTree`, `AggregatingMergeTree`) after stripping `Replicated` or `Shared` | `system.tables.engine` | refusal |
| table | replication mode against `table_engine`: a local engine under `replicated`, or a `Replicated*` engine under `merge_tree`, is a mismatch; `Shared*` satisfies either, because ClickHouse Cloud substitutes it for both | `system.tables.engine` | refusal |
| table | sorting key, partition key, primary key, sampling key | `system.tables` | refusal |
| table | TTL clause, taken from `engine_full`, which carries `TTL ... SETTINGS ...` without the column list | `system.tables` | `MODIFY TTL` or `REMOVE TTL` (deferred) |
| table | constraints and projections declared by the template | `create_table_query` | refusal when missing live |
| table | settings, taken from `engine_full` after `SETTINGS` | `system.tables` | `MODIFY SETTING` (deferred) |
| column | exists | `system.columns` | `ADD COLUMN IF NOT EXISTS ... AFTER` (additive) |
| column | type | `system.columns.type` | refusal |
| column | default expression, for the plain `DEFAULT` kind | `system.columns` | `MODIFY COLUMN ... DEFAULT`, or `REMOVE DEFAULT` when the template has none (deferred) |
| column | default kind (`DEFAULT`, `MATERIALIZED`, `ALIAS`, `EPHEMERAL`) | `system.columns.default_kind` | refusal |
| column | codec | `system.columns.compression_codec` | `MODIFY COLUMN ... CODEC` (deferred) |
| index | exists | `system.data_skipping_indices` | `ADD INDEX IF NOT EXISTS` (additive) |
| index | `type_full`, `expr`, `granularity` | `system.data_skipping_indices` | `DROP INDEX` then `ADD INDEX` (deferred) |
| view | exists | `system.tables` | `CREATE MATERIALIZED VIEW IF NOT EXISTS` (additive) |
| view | `TO` table | `create_table_query` | refusal |
| view | query | `system.tables.as_select` | `MODIFY QUERY` with the template's text (deferred) |

Deliberately not compared: the Keeper path and replica name of a `Replicated*` engine, because deployments legitimately carry paths no template reproduces; a view's declared columns, which `MODIFY QUERY` does not refresh; the position of existing columns; comments; and any column, index, projection or constraint present live and absent from the template, which an operator may have added and the reconciler never removes. Database qualifiers are stripped only where they name the scratch database or the configured database, so a deliberate cross-database reference would survive. `Nested` groups are flattened the same way on both sides, and a `Nested` group inside another is one `Array(Nested(...))` column, so a change inside it is a type difference.

The engine is never changed, because ClickHouse cannot change it in place (§2.3); the two engine rows exist to catch a configuration that disagrees with the tables, `table_engine.replicated` over local tables or `merge_tree` over replicated ones, with `SharedMergeTree` counting as replicated because ClickHouse Cloud stores every `MergeTree` table that way.

#### Additive and deferred statements

Statements fall into two classes, and the class decides who runs them.

- **Additive**: `CREATE ... IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `ADD INDEX IF NOT EXISTS`. Each is idempotent, and two such statements for the same object from different releases either agree or leave the one that ran first in place, after which the other definition shows up on the next `plan` as a deferred difference, or as a refusal if the two releases disagree on a type, which rule 2 of §5.4 forbids a release to do; nothing is lost and nothing is silently certified. Additive statements therefore need no coordination, and startup applies them.
- **Deferred**: `MODIFY COLUMN`, `MODIFY SETTING`, `MODIFY TTL`, `REMOVE TTL`, `DROP INDEX` plus `ADD INDEX`, `MODIFY QUERY`. These do not commute: an older release's `MODIFY QUERY` landing after a newer one's computes derived rows with the wrong query until someone notices, and an older TTL can expire parts in the meantime. ClickHouse has no primitive that lets concurrent instances order such statements safely: a distributed DDL that outlives its lease keeps executing, a `Replicated` database retries remote DDL until it completes, and a lock cannot revoke queued work. Deferred statements are therefore never applied at startup. They are reported, and applied by the one-shot operator command of §5.8, which an operator runs from one place at a time, or by startup under the opt-in `apply_deferred_changes: true`, which a deployment sets only when it guarantees by construction that a single instance reconciles, such as a single-node installation or an init container.

### 5.3 Startup behavior

| `create_schema` | Plan | Behavior |
| --- | --- | --- |
| `true` | empty | Start. |
| `true` | additive statements, possibly with deferred ones | Apply the additive statements, verify them (§5.6), log the deferred ones with the operator command that applies them, and start. With `apply_deferred_changes: true`, apply the deferred ones too. |
| `true` | any refusal | Fail with the refusals and the statements an operator would run. A startup step does not rewrite data. |
| `false` | empty | Start. |
| `false` | anything | Log the plan as a warning, statement by statement, and start. The warning names the command of §5.8. |
| `false` | cannot be computed | Warn and start. A deployment whose Jaeger user cannot create a scratch database ran without DDL privileges before this mechanism and continues to; the warning says which privilege is missing and that `clickhouse-schema plan` can be run with a privileged user instead. |
| `true` | cannot be computed | Fail: creating the schema was asked for and needs the privileges to plan it. |

A deployment on the previous release has nothing to bootstrap: the first start with this mechanism computes a plan against its tables like any other.

### 5.4 Rules for changing the schema

1. **Expand only.** A template edit adds columns, indexes, codecs and settings or changes a view's query. It never removes or renames a column that the previous release reads or writes; removal is a template edit in the release after the one that stopped using it, and even then the reconciler does not drop it, so an operator runbook does. A rolling deploy therefore always has an older binary running against a schema that is a superset of what it expects, which `INSERT` with an explicit column list and `SELECT` with explicit columns already tolerate.
2. **No data-rewriting changes at startup.** A column type, including `LowCardinality` and any change inside the nested attribute groups of `events` and `links`, a sorting key, a partition key and an engine are changed by operator procedures with runbooks. A TTL change is applied without materialization (§2.3) and is the exception. A template edit that changes one of them is legitimate for fresh installs, and the reconciler reports it as a refusal against existing databases, so the PR that makes the edit ships the runbook, and the test in §5.9 that reconciles from every prior release fails until it does.
3. **A view change is forward-only and must leave old rows valid.** `MODIFY QUERY` and `CREATE` of a missing view affect rows inserted from then on and repair nothing already in the target table. A view edit may therefore add derived columns, which old rows carry at their defaults, or change the query where existing target rows stay correct under the new definition. A change to grouping, filtering or aggregation that makes old rows wrong is not a template edit on its own: it ships with a backfill runbook that repopulates the target from `spans`, and the PR says so. Never drop-and-recreate a view, because inserts during the gap would bypass it.
4. **New columns have defaults the old binary can satisfy.** A new column takes its type default, so an `INSERT` from a binary that does not know the column still succeeds.

### 5.5 Concurrency

Startup applies only additive statements (§5.2), each idempotent, in template order. Concurrent instances of one release produce the same schema, and ClickHouse serializes `ALTER`s per table, so a duplicate `ADD COLUMN` is a no-op. Concurrent instances of two releases produce the union of their additive statements; where the two releases define a new column or index differently, the first definition stands and the second appears in every later `plan`, as a deferred difference for a codec, default or index definition and as a refusal for a type, so the disagreement is visible rather than resolved by chance. Additive statements commute up to that visible difference, which is what footnote 8 of §4.6 claims and no more. This is the whole concurrency story for startup, and it holds without a lock, a version table or a clock.

The operator command that applies deferred statements is meant to run once, from one place. As a guard rail against two operators running it at once, it takes a lock on a server that offers `KeeperMap` (§2.4): a `jaeger_schema_lock` table with one row keyed by the database name whose value is the holder's identity and a lease expiry, every mutation of which runs with `keeper_map_strict_mode = 1` and a predicate on the current value, so acquisition, renewal, takeover of an expired lease and release are each a compare-and-set that fails rather than overwrites. The lock serializes the command's own statements; it cannot revoke distributed DDL that a previous holder queued, which is why it is a guard rail and the single-run instruction is the guarantee. Where `KeeperMap` is unavailable the command says so and runs unlocked.

### 5.6 Replicated clusters

The plan is computed against the replica the connection reached. Column, index, codec and TTL `ALTER`s propagate through Keeper; `CREATE`, `MODIFY SETTING` and a view's `MODIFY QUERY` are local to that replica (§2.3). A multi-replica deployment therefore needs DDL fan-out for the reconciler to converge every replica in one pass. The backend adds an optional `cluster` field to `table_engine.replicated`; when set, every DDL statement the reconciler issues against the live database carries `ON CLUSTER <name>` through a template parameter, and the catalogs are read through `clusterAllReplicas(<name>, system.tables)` and its siblings so that a replica-local property, a setting or a view query, that drifted on a replica other than the connected one still produces a statement. The design assumes the single-shard layout the backend writes to today; the scratch database stays on the connected server. When unset, the operator is expected to have a `Replicated` database, which is the ClickHouse Cloud default, and the documentation says so; DDL is then issued with `distributed_ddl_output_mode = 'throw_only_active'`, which waits for the replicas that are up and fails if one of them does not apply the statement, rather than `throw`, which would wait the full `distributed_ddl_task_timeout` for a replica that is down and then fail every restart while it stays down; the setting is load-bearing because ClickHouse Cloud's default is `none`, which returns before the other replicas have applied anything. After the additive statements, and before the backend serves traffic, `plan` runs once more, through `clusterAllReplicas` when `cluster` is set, and any additive statement still outstanding fails startup under `create_schema: true`, because a replica that still lacks a column would fail the first insert routed to it; deferred differences in that second plan are logged, not fatal. A `Replicated` database rejects `Replicated*` engines with explicit arguments by default since ClickHouse 24.9, so in that deployment `table_engine.replicated` is used without `keeper_path` and `replica_name` and the database supplies both, which is the case #9813 left those fields optional for. Without either, a replica that was missed is found to lack its objects or settings by the next instance that connects to it and reconciled then, so the cluster converges over restarts rather than in one pass; that is the failure mode today's startup already has, and the versioned designs make it worse because a recorded version hides it.

### 5.7 Release snapshots and migration files

The diff engine also runs offline, against two sets of rendered templates rather than a live database, by creating both in scratch databases of a CI ClickHouse. A `make clickhouse-schema-snapshot` target writes `internal/storage/v2/clickhouse/sql/snapshots/<release>.sql`, the rendered schema with default parameters, and `migrations/<previous>_to_<release>.sql`, the plan from the previous snapshot to the new one, with refusals as comments naming the runbook. Both are committed by the release PR. They are documentation and audit trail, rendered with default parameters and therefore not executable against a deployment with its own `ttl`, `table_engine` or `cluster`: the binary never reads them, each file says so in its header, and the statements an operator runs come from `clickhouse-schema plan`, which renders with the deployment's configuration.

### 5.8 Operator path

```
jaeger ... --config config.yaml clickhouse-schema plan      # the plan against the live database, apply nothing
jaeger ... --config config.yaml clickhouse-schema apply     # apply the whole plan, additive and deferred, once
jaeger ... --config config.yaml clickhouse-schema render    # the rendered desired schema
```

`plan` is the `create_schema: false` warning on demand, exits non-zero when the plan is non-empty so it serves as a CI check, and prints refusals with the statements an operator would run. `apply` is the one-shot procedure of §5.5: it takes the lock where the server offers one, applies additive and deferred statements in template order, verifies, and refuses to run when the plan carries a refusal. It is also the init-container form for fleets that want the schema settled before pods roll. All three commands render with the configuration file's `ttl`, `table_engine` and `cluster`, so the output matches the deployment rather than a default. The root `jaeger` command is the collector's and owns `--config`, so the subcommand reads the configuration through the same loader rather than declaring its own flag.

### 5.9 Tests

All against the live ClickHouse that the integration suite already starts.

1. **Fresh install.** Startup on an empty database, then `plan` is empty, and every compared property (§5.2) of every object equals the rendered template as the server normalizes it.
2. **Upgrade from every release.** For each snapshot in `snapshots/`, apply it to an empty database, start, then run `clickhouse-schema apply`, and require the compared properties to equal a fresh install's and the plan to be empty afterwards. A template edit that needs a runbook fails this test with a refusal until the runbook exists and the test lists the change as expected.
3. **Idempotence and concurrency.** Start twice; start from two connections at once; the plan is empty after each. Start two binaries whose templates define a new column with different codecs at once and require the column to exist with one of the two codecs and the other to appear as a deferred difference in the next `plan`. Repeat for an index definition and a view query, and for a type, where the next `plan` is a refusal. Start two binaries against an empty database where one defines an extra column and require both columns present after both have started.
4. **Refusals.** For each refused row of the §5.2 table, create an object that differs from the template in that one way and require `plan` to return the refusal and startup to fail without executing anything. Include a local table under `table_engine.replicated` and a replicated table under `merge_tree`.
5. **Deferred statements.** Each deferred row of the §5.2 table: startup leaves the difference in place and logs it; `clickhouse-schema apply` resolves it; `apply_deferred_changes: true` resolves it at startup.
6. **Diff engine unit tests** on synthetic catalog rows: every row of the §5.2 table, the ignored list, `Nested` flattening, column placement, cross-object ordering, TTL clause extraction and qualifier stripping.
7. **Lock and replication.** Against a ClickHouse started with its embedded Keeper, which a single server supports: two `clickhouse-schema apply` runs at once leave one waiting, an expired lease is taken over exactly once, a renewal racing a takeover loses, a release racing a successor leaves the successor's row, and `KeeperMap` absent runs unlocked with a notice. A two-replica Compose job covers convergence through `ON CLUSTER` and through a `Replicated` database, a replica that is down during startup, and the verification failing startup when an additive statement did not reach a replica.

Run once with `merge_tree` and `ttl: 0` and once with `ttl` set, so the TTL clause is covered live. The CI ClickHouse gains an embedded Keeper for test 7, and the Compose job runs in the storage integration workflow with the other backends.

---

## 6. Upgrading an Existing Deployment

An existing deployment has the twelve objects as the previous release created them. On the first start of a binary with this mechanism:

- With `create_schema: true`, `plan` finds the ten OTLP columns missing from `spans`, all additive, and startup adds them. No operator action, which is the point of putting the mechanism behind the existing flag.
- With `create_schema: false`, the binary logs the ten `ADD COLUMN` statements as a warning and starts; the writer fails on insert until the operator applies them, exactly as today, but the operator now has the statements in the log and in `clickhouse-schema plan`.

A mismatch between the configuration and what was created a year ago is detected, which is new: today it is invisible. A `ttl` that was since changed becomes a deferred `MODIFY TTL` without materialization, logged at startup and applied by `clickhouse-schema apply`, after which the configured retention takes effect as parts merge. A table whose engine disagrees with the configuration, say a local `MergeTree` under a configuration that now says `replicated`, is a refusal that stops startup with `create_schema: true` until the operator either rebuilds the table or corrects the configuration. This is the one way the upgrade can be noisier than before, and it is noisy about a real discrepancy.

---

## 7. Implementation Plan

1. **M1 — Diff engine and plan.** `schemaReconciler.plan`, the scratch-database method, the vocabulary and refusals, the `clickhouse-schema plan` and `render` commands, and the `create_schema: false` warning. Read-only; no change to what `create_schema: true` does. Tests 4 and 5.
2. **M2 — Apply.** Startup applies additive statements behind `create_schema: true` per §5.3 and verifies them; `clickhouse-schema apply` applies the whole plan unlocked; `apply_deferred_changes` is added. Tests 1, 3 and 5. #9796 is superseded by M1 and M2. The guarantees that ship here are the ones §5.5 makes for additive statements; the lock guard rail for the operator command arrives in M5.
3. **M3 — OTLP span columns.** The ten columns added to `create_spans_table.sql`, the `SpanRow` and conversion wiring, the corpus fixture, and a live upgrade test from a snapshot of the schema as it stood before this milestone, taken from git; #9799 reduced to that. This is the first upgrade the mechanism performs for real, and it is tested live before it ships.
4. **M4 — Release snapshots.** The `make` target of §5.7, which also generalizes the M3 snapshot into test 2.
5. **M5 — Clusters.** The `cluster` field, `ON CLUSTER` rendering, `clusterAllReplicas` catalog reads, the `KeeperMap` lock for `clickhouse-schema apply`, and test 7.
6. **M6 — RFC 0017 changes** land as template edits through this mechanism, each as its own PR; the `LowCardinality` change ships with its runbook.

---

## 8. Open Questions

- **Refusal strictness with `create_schema: true`.** §5.3 fails startup on any refusal. An alternative is to warn and start when the refusal concerns only a property the binary can live with, such as a sorting key that differs from the template; the strict form is proposed because a silent discrepancy is what the mechanism exists to end.
- **Scratch database privileges.** `plan` needs `CREATE DATABASE` and `DROP DATABASE`. §5.3 lets an operator-managed deployment start without them; whether to offer a parser-based fallback that needs no scratch database is deferred until a user needs it.
- **Minimum ClickHouse version.** `MODIFY QUERY` left its experimental setting in 23.11 and `keeper_map_strict_mode` is older than that. The backend has stated no floor so far; 24.3 is proposed and would be documented with M1. One fact bears on it: the TTL clause the templates render today, `start_time + INTERVAL n SECOND` over a `DateTime64(9)` column, is rejected with `BAD_TTL_EXPRESSION` on 24.8 and 25.3 and accepted on 25.12, so either the floor is 25.12 or the template wraps the column in `toDateTime(...)`. The template fix is proposed, as its own change before M1, because it widens compatibility and is a deferred TTL difference for existing databases rather than a rewrite.
- **`apply_deferred_changes` as a setting.** It exists so that a single-node installation keeps today's hands-off behavior for every change. The alternative is to have no such setting and ask even single-node operators to run `clickhouse-schema apply` for deferred changes, which is simpler and one more step.

---

## References

1. [golang-migrate/migrate](https://github.com/golang-migrate/migrate): the migration library examined in §4.1, at v4.20.1.
2. [ClickHouse ALTER TABLE reference](https://clickhouse.com/docs/sql-reference/statements/alter): which changes are metadata-only and which are mutations.
3. [ClickHouse `ALTER TABLE ... MODIFY TTL`](https://clickhouse.com/docs/sql-reference/statements/alter/ttl): why TTL changes are mutations.
4. [ClickHouse `KeeperMap` table engine](https://clickhouse.com/docs/engines/table-engines/special/keeper-map): the strict-mode insert that serves as a lock.
5. [ClickHouse `Replicated` database engine](https://clickhouse.com/docs/engines/database-engines/replicated): DDL replication across replicas without `ON CLUSTER`.
6. [Schema migration tools for ClickHouse](https://clickhouse.com/docs/resources/support-center/knowledge-base/tables-schema/schema-migration-tools): ClickHouse's own survey, including Atlas's licensing.
7. [Automatic Migrations for ClickHouse with Atlas](https://atlasgo.io/guides/clickhouse): the declarative workflow on ClickHouse.
8. [PostHog/chschema](https://github.com/PostHog/chschema), [Noibu/housekeeper](https://github.com/Noibu/housekeeper), [anytoe/chsync](https://github.com/anytoe/chsync), [zifter/clickhouse-migrations](https://github.com/zifter/clickhouse-migrations): the third-party reconcilers surveyed in §4.5.
9. [Jaeger Cassandra schema management](../../internal/storage/v1/cassandra/schema/README.md): the per-version full-copy layout this RFC avoids.
10. [Issue #9789](https://github.com/jaegertracing/jaeger/issues/9789): the OTLP column gap and the original migration sketch.

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

Reconciliation is the recommendation. It keeps today's per-object templates as the living source of truth, needs no version numbering, frozen files or golden dumps, and gives operators who manage the schema themselves a printed plan of exactly what their database lacks. ClickHouse makes the diff tractable without a SQL parser: the desired objects are created in a scratch database and compared with the live ones through the system catalogs, so both sides are normalized by the same server. The engine refuses every change that would rewrite data and never drops anything, which keeps a startup step metadata-only. A survey of third-party reconcilers for ClickHouse found none that is both embeddable and under a free license, so the diff engine is written in-house, informed by the designs of those tools. Release-time snapshots and numbered migration files are produced from the same engine for people who want a versioned audit trail.

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

The schema is twelve files under [`internal/storage/v2/clickhouse/sql/`](../../internal/storage/v2/clickhouse/sql/), one per object, each embedded as a string constant in `queries.go`:

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
| Change a column's type, including adding `LowCardinality` | `ALTER TABLE ... MODIFY COLUMN ... <type>` | a mutation that rewrites every part² | propagates |
| Change a table setting | `ALTER TABLE ... MODIFY SETTING` | metadata only | propagates |
| Change TTL | `ALTER TABLE ... MODIFY TTL` | a mutation that rewrites the parts the new expression expires² | propagates |
| Change a view's query | `ALTER TABLE <view> MODIFY QUERY` | metadata only | propagates |
| Extend the sorting key | `ALTER TABLE ... MODIFY ORDER BY` | metadata only, and only by appending columns added in the same statement | propagates |
| Drop a column or index | `ALTER TABLE ... DROP COLUMN` | destroys data | propagates |
| Change `ENGINE`, `PARTITION BY`, or the existing sorting key | not possible | requires a new table and a copy | n/a |

- ¹ The column is added to each part's metadata; existing parts read the column's default until they are merged.
- ² A mutation is background work over every affected part, which for a large `spans` table is hours of I/O. It is the reason TTL belongs in `CREATE` and not in a per-startup `ALTER`, and the reason type changes are operator procedures rather than startup steps (§5.4).

Two properties of replicated deployments matter. An `ALTER` on a `Replicated*` table is recorded in Keeper and applied on every replica, so a statement issued through one connection reaches the whole shard. A `CREATE TABLE` is not: it creates the table only on the server the client reached, and reaching the others needs `ON CLUSTER`, a database with the `Replicated` database engine, or a connection to each. §5.6 draws the consequence.

### 2.4 Facilities ClickHouse offers a schema tool

- **System catalogs.** `system.tables` has `engine_full`, `sorting_key`, `partition_key`, `create_table_query` and, for views, `as_select`; `system.columns` has each column's `type`, `default_kind`, `default_expression`, `compression_codec` and `position`, with `Nested` groups flattened to `events.name Array(String)`; `system.data_skipping_indices` has each index's `type_full`, `expr` and `granularity`. Everything a diff needs is a query away, normalized by the server.
- **Scratch databases.** `CREATE DATABASE` is cheap, so the desired DDL can be created under a throwaway database and compared with the live objects through the catalogs above, which avoids writing a ClickHouse DDL parser.
- **A cross-process lock.** ClickHouse has no advisory locks, but a `KeeperMap` table with `keeper_map_strict_mode = 1` turns an `INSERT` of an existing key into an error, which is a lock with Keeper as the arbiter. It needs a Keeper ensemble and a `keeper_map_path_prefix` in the server configuration, both of which a replicated deployment has.
- **DDL fan-out.** The `Replicated` database engine replays every DDL statement issued to one replica on all of them, and is the default on ClickHouse Cloud; `ON CLUSTER <name>` does the same per statement on a self-hosted cluster with an ordinary `Atomic` database.

### 2.5 How the other backends do it

Cassandra was described above; its v2 backend also creates the schema from the Go template at startup, but that path can only create, not migrate. Elasticsearch and OpenSearch have no comparable problem: their mapping is an index template that the binary installs at startup, and because indices roll, by day or by alias rollover, a new mapping applies to the next index without altering the previous one. Badger and memory have no persistent schema. ClickHouse is the first Jaeger backend whose tables are long-lived and alterable, which is why neither precedent transfers.

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

This is the model of Flyway, Liquibase, golang-migrate and goose, and the one #9789 sketched: a numbered sequence of forward-only files, a table recording which have run, and a library that applies the pending ones in order. The current schema is the sum of the files, so the baseline is version 1 and every change is a new file. #9796 implements it with golang-migrate, putting the baseline in one `000001_initial_schema.up.sql` rendered through the templates and the version table on the configured engine.

The library's contract is narrower than its examples suggest. Its `source.Driver` interface is seven methods that enumerate integer versions and return one `io.Reader` of SQL per version per direction; the `{N}_{name}.up.sql` filename rule belongs to the `source/iofs` adapter, which lists one directory of any `fs.FS`, so per-object files can be rendered and concatenated into an in-memory `fs.FS` and the single-file layout is a choice, not a constraint. The `database/clickhouse` driver takes a `*sql.DB` and a `Config` with `MigrationsTable`, `MigrationsTableEngine` (default `TinyLog`), `ClusterName` (applied to the version table's DDL only) and `MultiStatementEnabled`. The following behaviors, checked at v4.20.1, are what a Jaeger integration would have to work around:

- `WithInstance` creates the version table as a side effect, so a read-only check of the version has to bypass the library.
- The version table is created with `CREATE TABLE` without `IF NOT EXISTS` after a `SHOW TABLES` check, so two instances starting against an empty database race and the loser fails before migrating.
- `ORDER BY sequence` is appended to the version table's DDL only when `MigrationsTableEngine` ends in `Tree`. A replicated engine clause with explicit arguments, `ReplicatedMergeTree('/path', 'replica')`, ends in `)`, so the driver emits a `MergeTree`-family table without a sorting key, which ClickHouse rejects.
- `Up()` fails when the recorded version is absent from the source, so a binary rolled back one release cannot start against a database one version ahead, which defeats the expand-before-contract rule that makes rollback possible.
- The driver only accepts `*sql.DB`, so the backend would open a second connection through `database/sql` beside the native `driver.Conn` it already holds.
- `Lock()` is a process-local flag, so concurrency rests entirely on every statement being idempotent, and even then two instances with several versions pending can interleave them: one applies `v003` before the other finishes `v002`, and if both versions set the same property the older value wins while the newer version is recorded.

With one statement per file, no down files and idempotent statements, what the library still contributes is the version table and the ordered walk, about a hundred lines, and the familiarity of its table shape.

### 4.2 Versioned deltas applied by an in-house migrator

The same layout, one directory per version holding one file per object the version changes, with the walk and the version table written in the backend over the native connection. The version table is created with `IF NOT EXISTS` on the configured engine with an explicit sorting key, a crashed version is finished by the next start because every statement is idempotent, a newer recorded version is a warning rather than a failure, and there is no second connection. Every quirk in the list above disappears except the last, the cross-version interleaving, which needs either the `KeeperMap` lock of §2.4 or a rule that a version never reassigns a property an earlier version assigned.

The bookkeeping stays: version numbers, a frozen baseline that is no longer the readable schema, a golden `SHOW CREATE TABLE` dump to read instead, and a rule that a released version is never edited.

### 4.3 Full copy of the schema per version

The Cassandra model. Each version is a complete, readable schema, and the diff between two versions is whatever a human writes into a migration script. It fails requirement 1 by construction and records nothing in the database, and is listed because it is the precedent in this repository.

### 4.4 Living templates for fresh installs, deltas for upgrades

Keep today's templates as the readable current schema and use them for a fresh install, recording the newest version number without running the deltas; an existing database applies the deltas. This scores well on readability and loses on requirement 2: the templates and the deltas are two descriptions of the same schema, and only a parity test keeps them equal. A template edit with no matching delta passes review and breaks upgrades silently.

### 4.5 Reconciliation

Describe the desired schema once, declaratively, and let the tool compute the statements that bring the live database to it. This is how Kubernetes controllers treat resources, how Terraform treats infrastructure, and how Atlas, Skeema, sqldef and Prisma treat relational schemas. For ClickHouse specifically it is the model of Atlas's ClickHouse driver, PostHog's `chschema`, Noibu's `housekeeper` and the `diff` command of the Python `clickhouse-migrations`.

For this backend the desired schema is today's twelve templates, unchanged and still edited in place. At startup the backend renders them, creates each object under a scratch database, reads both the scratch and the live objects from the system catalogs, and diffs them per object. The diff vocabulary is exactly the metadata-only rows of §2.3: `ADD COLUMN`, `ADD INDEX`, `MODIFY COLUMN ... CODEC`, `MODIFY SETTING`, `MODIFY QUERY`, and `CREATE` for an object that does not exist. Anything else the diff detects, a type change, a TTL change, a different sorting key, partition key or engine family, is reported and refused, because each is a mutation or a rebuild. Anything present in the live table and absent from the desired schema is left alone, because drop versus rename is undecidable from a diff and either loses data. The plan is a list of statements; with `create_schema: true` the backend applies it, with `create_schema: false` it prints it and starts, and a command prints it on demand.

Two properties follow that no versioned design has. The templates are the single readable source of truth, with no numbering, no frozen files and no golden dump. And a database that drifted, because an operator added an index by hand or an earlier version of the binary created a table slightly differently, is reconciled toward the desired state rather than assumed to match a version number.

A versioned audit trail is a product of the same engine rather than its input: at release time, diffing the previous release's rendered schema against the current one through a scratch database yields a numbered migration file to commit, for people who want one.

#### Third-party reconcilers for ClickHouse

The engine would be a few hundred lines, so a library was looked for first. None is both embeddable in a Go binary and under a free license.

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
| 9. Little bookkeeping | ❌ | ❌ | 🟨 | ❌ | 🟢 |
| 10. Production statements reviewable | 🟢 | 🟢 | 🟨 | 🟢 | 🟨¹³ |
| Code to own | 🟢 glue only | 🟨 ~100 lines | 🟢 | 🟨 | ❌ ~400 lines |

- ¹ Deltas from version 2 on, but the baseline is one long file and the per-object organization is lost when the framework is adopted.
- ² Templates and deltas are two descriptions kept equal only by a parity test.
- ³ As #9796 stands it renders the baseline through the templates; the earlier shape its review commented on hard-coded the engine.
- ⁴ A version number, which says what was recorded, not what the tables contain; drift is invisible.
- ⁵ The plan is computed from the live catalog, so what is reported is what the database actually lacks.
- ⁶ The version table's `CREATE` races on first start, and pending versions can interleave across instances (§4.1).
- ⁷ Correct if every statement is idempotent and no version reassigns an earlier version's property; a `KeeperMap` lock would make it 🟢.
- ⁸ Every statement is idempotent and there is no shared bookkeeping to race on; the optional `KeeperMap` lock removes even the duplicate work.
- ⁹ The operator receives "versions 2 and 3 are pending" and the files, and is trusted to apply them whole.
- ¹⁰ The operator receives the exact statements their database lacks, computed against it.
- ¹¹ Through a generated golden dump rather than the authored files.
- ¹² A second `database/sql` connection for the library's driver.
- ¹³ The PR shows the template edit, not the `ALTER`; the statements are visible in the release-time migration file (§5.7) and in the printed plan.

Reconciliation is the recommendation. The rows that decided it are 4, 6, 7 and 9: the plan is computed from what the database actually contains, the operator gets the exact statements they lack, the templates stay the readable schema, and contributors learn no numbering or freezing rules. The row it loses, code to own, is a few hundred lines of catalog comparison that the prior art shows to be a solved shape, and row 10 is recovered by the release-time migration files. The in-house migrator of §4.2 is the fallback if the diff engine proves harder than estimated; it shares the per-object layout and the idempotence rules, so the templates and tests carry over.

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
// the live objects match. Statements are grouped as applicable or refused.
func (r *schemaReconciler) plan(ctx context.Context) (schemaPlan, error)

// apply executes the applicable statements of a plan in order and fails if any
// statement is refused.
func (r *schemaReconciler) apply(ctx context.Context, p schemaPlan) error

type schemaPlan struct {
    Create   []string // CREATE for objects that do not exist
    Alter    []string // metadata-only ALTERs
    Refused  []schemaRefusal
}

type schemaRefusal struct {
    Object    string
    Reason    string // e.g. "column duration: type Int64 differs from UInt64"
    Statement string // what an operator would run, for the message
}
```

`plan` does the following, all through the native connection the factory already holds:

1. Create a scratch database `jaeger_schema_<random>`, render each template and create its object there. Scratch tables are rendered with the local engine family regardless of `table_engine`, because the engine is never diffed and a `Replicated*` scratch table would register a Keeper path.
2. For each desired object, read `system.tables`, `system.columns` and `system.data_skipping_indices` for both the scratch and the live copy. `Nested` columns are flattened the same way on both sides.
3. Per object, compute the difference. A live object that is missing yields its rendered `CREATE` with the deployment's engine. A missing column yields `ADD COLUMN IF NOT EXISTS` placed `AFTER` its predecessor; a missing index yields `ADD INDEX IF NOT EXISTS`; a codec difference yields `MODIFY COLUMN ... CODEC`; a setting difference yields `MODIFY SETTING`; a view whose `as_select` differs yields `MODIFY QUERY`. A type difference, a TTL difference (compared on the `TTL` clause of the normalized `create_table_query`), a sorting key, partition key or engine family difference yields a refusal. Columns and indexes present live and absent from the desired schema yield nothing.
4. Drop the scratch database, also on error.

Idempotence is a property of the vocabulary: every applicable statement is a no-op when the live object already matches, so two instances applying the same plan both succeed, and a plan computed after `apply` is empty.

### 5.3 Startup behavior

| `create_schema` | Plan | Behavior |
| --- | --- | --- |
| `true` | empty | Start. |
| `true` | applicable statements only | `apply`, then start. |
| `true` | any refusal | Fail with the refusals and the statements an operator would run. A startup step does not rewrite data. |
| `false` | empty | Start. |
| `false` | anything | Log the plan as a warning, statement by statement, and start. The warning names the command of §5.8. |

A deployment on the previous release has no version table to migrate and nothing to bootstrap: the first start with this mechanism computes a plan against its tables like any other.

### 5.4 Rules for changing the schema

1. **Expand only.** A template edit adds columns, indexes, codecs and settings or changes a view's query. It never removes or renames a column that the previous release reads or writes; removal is a template edit in the release after the one that stopped using it, and even then the reconciler does not drop it, so an operator runbook does. A rolling deploy therefore always has an older binary running against a schema that is a superset of what it expects, which `INSERT` with an explicit column list and `SELECT` with explicit columns already tolerate.
2. **No data-rewriting changes at startup.** A column type, including `LowCardinality`, a TTL expression, a sorting key, a partition key and an engine are changed by operator procedures with runbooks. A template edit that changes one of them is legitimate for fresh installs, and the reconciler reports it as a refusal against existing databases, so the PR that makes the edit ships the runbook, and the test in §5.9 that reconciles from every prior release fails until it does.
3. **Views change by `MODIFY QUERY`.** Never drop-and-recreate, because inserts during the gap would bypass the view.
4. **New columns have defaults the old binary can satisfy.** A new column takes its type default, so an `INSERT` from a binary that does not know the column still succeeds.

### 5.5 Concurrency

Every statement in the vocabulary is idempotent and the plan has no cross-statement dependencies beyond order within one object, so concurrent instances applying the same plan produce the same schema. Two instances may both issue the same `ADD COLUMN`, and ClickHouse serializes `ALTER`s per table, so the second is a no-op.

Where a Keeper ensemble exists, which is every deployment with `table_engine.replicated`, the reconciler additionally takes the `KeeperMap` lock of §2.4 around `plan` and `apply`: a `jaeger_schema_lock` table with `keeper_map_strict_mode = 1`, one row whose key is the database name and whose value is the instance identity and a timestamp, released after `apply` and treated as stale after 60 s. An instance that cannot take the lock waits and recomputes its plan afterwards, which is then empty. The lock removes duplicate work and the drift between replicas' views of an in-flight `ALTER`; it is not what correctness rests on.

### 5.6 Replicated clusters

The plan is computed against the replica the connection reached. `ALTER` statements propagate through Keeper and need nothing more. `CREATE` statements are local (§2.3), so a fresh database on a self-hosted cluster whose replicas are not each reached by a Jaeger instance needs DDL fan-out. The backend adds an optional `cluster` field to `table_engine.replicated`; when set, every statement the reconciler issues carries `ON CLUSTER <name>` through a template parameter. When unset, the operator is expected to have a `Replicated` database, which is the ClickHouse Cloud default, and the documentation says so. Because the reconciler plans against the live catalog rather than a version number, a replica that was missed is simply found to lack its tables by the next instance that connects to it, which is the failure mode today's startup already has and the versioned designs make worse (§4.1).

### 5.7 Release snapshots and migration files

The diff engine also runs offline, against two sets of rendered templates rather than a live database, by creating both in scratch databases of a CI ClickHouse. A `make clickhouse-schema-snapshot` target writes `internal/storage/v2/clickhouse/sql/snapshots/<release>.sql`, the rendered schema with default parameters, and `migrations/<previous>_to_<release>.sql`, the plan from the previous snapshot to the new one, with refusals as comments naming the runbook. Both are committed by the release PR. They are documentation and audit trail: the binary never reads them, and an operator who prefers versioned files applies them instead of the printed plan.

### 5.8 Operator path

```
jaeger ... --config config.yaml clickhouse-schema plan      # the plan against the live database, apply nothing
jaeger ... --config config.yaml clickhouse-schema render    # the rendered desired schema
```

`plan` is the `create_schema: false` warning on demand, exits non-zero when the plan is non-empty so it serves as a CI check, and prints refusals with the statements an operator would run. Both commands render with the configuration file's `ttl`, `table_engine` and `cluster`, so the output matches the deployment rather than a default.

### 5.9 Tests

All against the live ClickHouse that the integration suite already starts.

1. **Fresh install.** `apply` on an empty database, then `plan` is empty, and `SHOW CREATE TABLE` of every object equals the rendered template as the server normalizes it.
2. **Upgrade from every release.** For each snapshot in `snapshots/`, apply it to an empty database, run `apply`, and require the resulting catalog to equal a fresh install's and the plan to be empty afterwards. A template edit that needs a runbook fails this test with a refusal until the runbook exists and the test lists the change as expected.
3. **Idempotence and concurrency.** `apply` twice; run `apply` from two connections at once; the plan is empty after each and the catalog equals a fresh install's.
4. **Refusals.** For each refused class, create a table that differs from the template in that one way and require `plan` to return the refusal and `apply` to fail without executing anything.
5. **Diff engine unit tests** on synthetic catalog rows: every vocabulary statement, `Nested` flattening, column placement, and TTL clause extraction.

Run once with `merge_tree` and `ttl: 0` and once with `ttl` set, so the TTL clause is covered live; the `replicated` and `ON CLUSTER` rendering and the `KeeperMap` lock are covered by unit tests, because the CI ClickHouse runs without Keeper.

---

## 6. Upgrading an Existing Deployment

An existing deployment has the twelve objects as the previous release created them. On the first start of a binary with this mechanism:

- With `create_schema: true`, `plan` finds the ten OTLP columns missing from `spans` and nothing else, and `apply` adds them. No operator action, which is the point of putting the mechanism behind the existing flag.
- With `create_schema: false`, the binary logs the ten `ADD COLUMN` statements as a warning and starts; the writer fails on insert until the operator applies them, exactly as today, but the operator now has the statements in the log and in `clickhouse-schema plan`.

A mismatch between the configuration and what was created a year ago, say a `ttl` that was since changed, is detected as a refusal, which is new: today it is invisible. With `create_schema: true` that refusal stops startup until the operator either runs the `MODIFY TTL` the message prints or sets `ttl` back to what the table has. This is the one way the upgrade can be noisier than before, and it is noisy about a real discrepancy.

---

## 7. Implementation Plan

1. **M1 — Diff engine and plan.** `schemaReconciler.plan`, the scratch-database method, the vocabulary and refusals, the `clickhouse-schema plan` and `render` commands, and the `create_schema: false` warning. Read-only; no change to what `create_schema: true` does. Tests 4 and 5.
2. **M2 — Apply.** `apply` replaces `schemaBuilder` behind `create_schema: true`, with the §5.3 table. Tests 1 and 3. #9796 is superseded by M1 and M2.
3. **M3 — OTLP span columns.** The ten columns added to `create_spans_table.sql`, the `SpanRow` and conversion wiring, and the corpus fixture; #9799 reduced to that. This is the first upgrade the mechanism performs for real.
4. **M4 — Release snapshots.** The `make` target of §5.7, the first snapshot taken from the release before M3, and test 2.
5. **M5 — Clusters.** The `cluster` field, `ON CLUSTER` rendering and the `KeeperMap` lock of §5.5.
6. **M6 — RFC 0017 changes** land as template edits through this mechanism, each as its own PR; the `LowCardinality` change ships with its runbook.

---

## 8. Open Questions

- **Refusal strictness with `create_schema: true`.** §5.3 fails startup on any refusal. An alternative is to warn and start when the refusal concerns only a property the binary can live with, such as a TTL that differs from the configuration; the strict form is proposed because a silent discrepancy is what the mechanism exists to end.
- **Scratch database privileges.** `plan` needs `CREATE DATABASE` and `DROP DATABASE`. A deployment whose Jaeger user lacks them can still run `clickhouse-schema render` and diff by hand, and the error message says so; whether to offer a parser-based fallback is deferred until a user needs it.
- **One-shot mode.** Large fleets may prefer an init container that applies the plan once before the pods roll. A `clickhouse-schema apply` command is a small addition to §5.8 if the demand appears.

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

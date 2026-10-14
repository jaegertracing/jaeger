# RFC 0019: ClickHouse Schema Migrations

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-10-13
- **Last Updated:** 2026-10-14
- **Issue:** [#9789](https://github.com/jaegertracing/jaeger/issues/9789)
- **Related:** [ADR-008 (ClickHouse storage schema)](../adr/008-clickhouse-storage-schema.md) · [RFC 0017 (schema tuning adopted from ClickStack)](./0017-clickhouse-schema-vs-clickstack.md) · [#9796 (golang-migrate PR)](https://github.com/jaegertracing/jaeger/pull/9796) · [#9799 (OTLP span columns PR)](https://github.com/jaegertracing/jaeger/pull/9799) · [#9813 (configurable table engine)](https://github.com/jaegertracing/jaeger/pull/9813)

---

## Abstract

The ClickHouse backend creates its twelve tables and materialized views at startup. It uses one `CREATE ... IF NOT EXISTS` template per object but cannot change an existing object. The missing OTLP span columns in #9789 are the first schema change since users received the schema. That change therefore has to establish how the schema is amended from now on.

This RFC describes the current mechanism and the requirements for its replacement. It analyses five families of approach. The first uses a migration framework to apply versioned delta files, as #9789 sketches and #9796 implements with golang-migrate. The second applies the same deltas through a small in-house migrator. The third keeps a full schema copy per version, as the Cassandra backend does. The fourth uses living templates for fresh installs and generated, reviewed deltas for upgrades, as Atlas and Prisma do. The fifth uses reconciliation: the templates remain the single declarative description of the desired schema, and the backend computes at startup the `ALTER` statements that bring the live database to it. Kubernetes controllers and Terraform use this approach.

This RFC recommends reconciliation. It keeps today's per-object templates as the living source of truth, with no version number or frozen file for a contributor to manage. Operators who manage their own schema receive a printed plan of exactly what their database lacks. ClickHouse makes this comparison tractable without a SQL parser. The reconciler creates the desired tables under throwaway names beside the live tables and compares both through the system catalogs. The same server therefore normalizes both sides.

At startup, the backend applies only the additive part of the plan: new objects, columns and indexes. These statements are idempotent and commute across concurrent instances and releases. The backend reports every other change to a codec, default, setting, TTL or view query. A one-shot operator command applies those changes. Startup can also apply them under an opt-in for deployments that ensure by construction that only one instance reconciles. This restriction is necessary because ClickHouse has no primitive that can safely serialize such changes across instances.

The engine reports changes that would rewrite data but never executes them. It never drops a table, view, column or index, and a difference it cannot resolve never causes startup to fail. A binary can therefore restart after an operator's runbook, and the previous release can restart during a rollback. A survey found no third-party ClickHouse reconciler that is both embeddable and under a free license. The diff engine is therefore written in-house, with those tools' designs as prior art. The same engine generates release-time snapshots and numbered migration files for people who want a versioned audit trail; contributors do not author them.

---

## 1. Motivation

Three conditions require a decision now.

1. **A schema change is pending.** #9789 adds ten columns to the `spans` table. Against an existing deployment, the startup statements are no-ops, so without a migration mechanism the columns never appear, and the writer fails on its first insert after the upgrade.
2. **RFC 0017 queues more.** Each codec, table setting and skip index adopted by RFC 0017 requires a metadata-only `ALTER TABLE`. Its `LowCardinality` wrappers change column types, which rewrites data and needs an operator procedure (§2.3). The backend will change its schema several times over the next few releases. Each change needs a defined path.
3. **Two PRs already implement one answer.** #9796 and #9799 adopt golang-migrate along the lines sketched in #9789. They place the entire baseline schema in `000001_initial_schema.up.sql`. Review of those PRs showed that the file layout, the test design and the library's fit need deciding before the code, which is what an RFC is for.

The Cassandra backend illustrates two problems to avoid. Its schema lives in `v001.cql.tmpl` through `v004.cql.tmpl` under [`internal/storage/v1/cassandra/schema/`](../../internal/storage/v1/cassandra/schema/), each a complete copy of every table. Upgrades are operator-run shell scripts in a `migration/` directory that the binary knows nothing about. Changing one table requires a new copy of the entire schema, and nothing records the version of a running keyspace. The ClickHouse mechanism has to improve on both properties.

---

## 2. Current State

### 2.1 Files

The schema has twelve files under [`internal/storage/v2/clickhouse/sql/`](../../internal/storage/v2/clickhouse/sql/), one per object. The `queries.go` file embeds each as a string variable:

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

The six table files are `text/template` documents, which `factory.go` renders with the `schemaTemplateParams` type:

```go
type schemaTemplateParams struct {
    TTLSeconds           int64  // zero renders no TTL clause
    MergeTree            string // ENGINE clause body for raw-row tables
    AggregatingMergeTree string // ENGINE clause body for derived tables
}
```

The `TTLSeconds` field comes from the `ttl` setting and renders `TTL start_time + INTERVAL n SECOND DELETE` on `spans` and `TTL end + ...` on `trace_id_timestamps`. The two engine strings come from the `table_engine` block that #9813 introduced: `merge_tree: {}` renders `MergeTree` and `AggregatingMergeTree`, and `replicated: {...}` renders `ReplicatedMergeTree('/path', 'replica')` and its aggregating counterpart. The materialized views use plain SQL because each writes `TO` its target table and has no engine of its own.

### 2.2 Startup behavior

The `newSchemaBuilder` function holds the twelve statements in dependency order, with tables before the views that read them. It renders the six templates. With `create_schema: true`, the `NewFactory` function executes each statement over the native connection before it serves traffic. With `create_schema: false`, nothing runs, and the operator owns the tables.

The backend records nothing about the statements it executes. It has no version table or DDL fingerprint, and startup does not check whether the tables match the binary's expectations. A mismatch causes an insert or query error on the first request that touches the missing column.

### 2.3 What ClickHouse allows after `CREATE`

The parts of a table that can change in place determine the shape of a migration mechanism.

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

- ¹ ClickHouse adds the column to each part's metadata. Existing parts have no value for it and evaluate the current default at read time until they merge. A later default change therefore changes the values read from those parts (§5.4).
- ² A mutation performs background work over every affected part. For a large `spans` table, this takes hours of I/O. Type changes therefore belong in operator procedures, rather than startup steps (§5.4).
- ³ A `MODIFY SETTING` on a `Replicated*` table changes only the replica that receives it. The server code establishes this behavior: the `StorageReplicatedMergeTree::alter` method does not replicate an alter that changes only settings. The ClickHouse documentation does not state this restriction. A materialized view uses the `MaterializedView` engine, rather than a `Replicated*` engine, so its `MODIFY QUERY` is also local. Reaching every replica requires the fan-out described in §2.4.
- ⁴ With materialization off, an existing part keeps its rows and the TTL bounds recorded under the old expression until a merge rewrites it. A shortened TTL therefore reclaims a part that no longer merges only after an operator runs `ALTER TABLE ... MATERIALIZE TTL`. A `spans` partition that no longer receives inserts is such a part.

Replication does not cover all DDL. Keeper records a column, index or codec `ALTER` on a `Replicated*` table, and every replica applies it. One connection therefore reaches the entire shard for these changes. In contrast, `CREATE TABLE`, `MODIFY SETTING` and a view's `MODIFY QUERY` affect only the server that the client reaches. Reaching the other servers requires `ON CLUSTER`, a database with the `Replicated` database engine, or a connection to each server. Section 5.6 applies this distinction to the design.

### 2.4 Facilities ClickHouse offers a schema tool

- **System catalogs.** The `system.tables` catalog provides `engine_full`, `sorting_key`, `partition_key`, `create_table_query` and, for views, `as_select`. The `system.columns` catalog provides each column's `type`, `default_kind`, `default_expression`, `compression_codec` and `position`. It flattens `Nested` groups to columns such as `events.name Array(String)`. The `system.data_skipping_indices` catalog provides each index's `type_full`, `expr` and `granularity`. Queries can therefore retrieve every property needed for the diff, normalized by the server.
- **Scratch tables.** Creating an empty table with `CREATE TABLE` is cheap. The tool can create the desired DDL under a throwaway name and compare it with live objects through the catalogs above. This avoids the need for a ClickHouse DDL parser. The `formatQuerySingleLine` function similarly normalizes views: it reprints a `SELECT` in the form stored by `as_select`. Scratch tables have to be ordinary persistent tables. A session-scoped `CREATE TEMPORARY TABLE` would need no cleanup, but its catalogs omit required properties. On ClickHouse 25.12, a temporary `MergeTree` table reports empty `sorting_key` and `partition_key` values and a bare `MergeTree` value for `engine_full` in `system.tables`. It reports none of its indexes in `system.data_skipping_indices`. Only its columns could therefore be compared.
- **A cross-process lock.** ClickHouse has no advisory locks. However, an `INSERT` into a `KeeperMap` table fails when the key exists if the query sets `keeper_map_strict_mode = 1`. This provides a compare-and-set operation with Keeper as the arbiter. It requires ClickHouse 23.4 or later, when `keeper_map_strict_mode` became available, and a Keeper ensemble. It also requires the opt-in `keeper_map_path_prefix` server setting, which no default configuration sets. Without that setting, the server refuses to create the table with `BAD_ARGUMENTS`. A strict-mode failure has no distinct error code: it returns `KEEPER_EXCEPTION` (error 999) with a "Node exists" message. The lock has to match that error and message.
- **DDL fan-out.** The `Replicated` database engine replays every DDL statement issued to one replica on all replicas. ClickHouse Cloud's `Shared` database engine builds on it and does the same for every database there. On a self-hosted cluster with an ordinary `Atomic` database, `ON CLUSTER <name>` provides the same fan-out per statement.

### 2.5 How the other backends do it

Section 1 covers Cassandra. Elasticsearch and OpenSearch have no comparable problem. The binary installs their mapping as an index template at startup. Indices roll by day or by alias rollover, so a new mapping applies to the next index without altering the previous one. Badger and memory have no persistent schema.

---

## 3. Requirements

This RFC judges the mechanism against the following criteria, which also form the comparison rows in §4.6.

1. **Per-object deltas.** Changing one column of one table changes one short file. It never requires a schema copy.
2. **One source of truth.** A fresh install and an upgraded install derive their final schema from one set of files. They do not depend on discipline to keep two paths equal.
3. **Template parameters survive.** The `ttl` and `table_engine` settings describe the deployment, rather than the schema. Every `CREATE` has to render them. A migration file that hard-codes `MergeTree` can silently give a replicated deployment local tables, which is a data-loss configuration.
4. **The database's state is knowable.** The binary can identify what the live schema lacks, and an operator can identify what a cluster has.
5. **Safe with concurrent instances.** Several Jaeger instances start together against one database. The result has to be correct regardless of which instance finishes second.
6. **Operator-managed schema stays possible.** With `create_schema: false`, the operator applies DDL by hand. The binary gives the operator exactly the DDL to apply.
7. **Readable current schema.** A developer changing the writer can read a file to see the current `spans` table. The developer does not have to replay its history.
8. **Small dependency footprint.** The backend adds no database drivers or connection paths that it does not otherwise use.
9. **Little bookkeeping imposed on contributors.** Each version number, ordering rule, frozen file and generated golden file adds something for a contributor to learn and a reviewer to check.
10. **What runs in production is reviewable.** A schema PR reviewer can see the statements that will execute. An operator can see them before an upgrade.
11. **Operator customizations survive.** An upgrade does not undo an index, column, codec or setting that an operator added by hand.
12. **Planning writes nothing.** Computing what the database lacks changes no live object and needs no privilege beyond catalog reads. This RFC states the privileges that a plan does need.

---

## 4. Approaches

### 4.1 Versioned deltas applied by a migration framework

Flyway, Liquibase, golang-migrate and goose use a numbered sequence of forward-only files. A table records which files have run, and a library applies pending files in order. This is the model sketched in #9789. The files together define the current schema: the baseline is version 1, and each change adds a file. #9796 implements this model with golang-migrate.

Two properties of golang-migrate affect this choice. First, the library does not constrain the layout. It reads each version as one `io.Reader` from any `fs.FS`, so the backend can render per-object files and concatenate them in memory. The single baseline file in #9796 is therefore a choice. Second, the ClickHouse driver fits this backend poorly. It requires a second connection through `database/sql`. Its process-local lock leaves concurrent instances dependent on idempotence alone. Its version-table handling and refusal to start against a newer recorded version each contradict a requirement in §3. The review of #9796 holds the specific objections. After workarounds for these problems, the library provides only the version table and the ordered walk, about a hundred lines of code.

### 4.2 Versioned deltas applied by an in-house migrator

An in-house migrator uses the same layout: one directory per version, with one file per object changed by that version. The backend implements the walk and version table over its native connection. This removes the driver problems above, but concurrent instances can still interleave changes from different versions. Preventing that requires the `KeeperMap` lock in §2.4 or a rule that no version reassigns a property assigned by an earlier version.

The bookkeeping also remains. Contributors manage version numbers and a frozen baseline that no longer describes the readable current schema. They read a golden `SHOW CREATE TABLE` dump instead and follow a rule that released versions never change. The ledger requires a checksum per applied file to detect edits to released versions. It also needs a baseline step for databases created with `create_schema: true` before the ledger existed. That step detects the presence of `spans` and records version 1 for those databases.

### 4.3 Full copy of the schema per version

Cassandra uses a complete, readable schema for each version. A human writes a migration script to express the difference between two versions. This model necessarily fails requirement 1 and records nothing in the database. It appears here because it is the repository's precedent.

### 4.4 Living templates for fresh installs, deltas for upgrades

This approach keeps today's templates as the readable current schema. A fresh install uses those templates and records the newest version number without running deltas. An existing database applies the deltas, which a PR commits alongside its template edit.

Atlas calls this the versioned workflow with `atlas migrate diff`; Prisma and Alembic use it too. A contributor edits the declarative schema. A tool compares it with the previous state in a throwaway database and writes a numbered migration file. The contributor reviews and commits that file rather than writing it.

In its strongest form, this approach applies only additive deltas at startup and leaves the rest to an operator command, exactly as §5 does. CI checks parity through the catalogs. It creates one database from the templates and compares it with a database created from each released snapshot plus the pending deltas. Both generation and parity checks use the catalog diff in §5.2, so this option also requires the diff engine. The difference is that production executes statements a reviewer read in the PR, rather than statements computed against the live catalog.

This approach has two costs. It falls short of requirement 2 because only the CI parity check detects a delta that was not regenerated after a later template edit. It also retains the version record, ordering rules and ledger upkeep of §4.2. A fleet deployed from trunk, with concurrent PRs, has to assign that ordering without a release number.

### 4.5 Reconciliation

Reconciliation describes the desired schema once, declaratively. The tool computes the statements needed to bring the live database to that state. Kubernetes controllers use this model for resources, Terraform uses it for infrastructure, and Atlas, Skeema, sqldef and Prisma use it for relational schemas. For ClickHouse, Atlas's ClickHouse driver, PostHog's `chschema`, Noibu's `housekeeper` and the Python `clickhouse-migrations` tool's `diff` command use this model. These tools run from a CLI at deploy time.

Reconcilers also have an established form that runs inside an application at startup, under the constraint that it only adds. Ent's `Schema.Create` method and GORM's `AutoMigrate` method inspect the live database. They issue `CREATE TABLE`, `ADD COLUMN` and `ADD INDEX` for objects present in the Go schema but absent from the database. Neither drops a column or index unless the caller passes an explicit option. The reason is that a reconciler that converges exactly would read a rename as a drop plus an add, and an older binary would drop the columns a newer one added. The design below uses the additive form.

For this backend, today's twelve templates describe the desired schema. They remain unchanged and are still edited in place. At startup, the backend renders them and creates each table under a throwaway name in the same database. It reads the scratch and live objects from the system catalogs and compares them per object.

The diff supports the statements in §2.3 that do not rewrite row data and can apply to any existing table. This excludes `MODIFY ORDER BY`, which can only append columns added in the same statement. The supported statements are `ADD COLUMN`, `ADD INDEX`, `DROP INDEX` followed by `ADD INDEX` to replace an index, `MODIFY COLUMN ... CODEC`, `MODIFY SETTING`, `MODIFY TTL`, `MODIFY QUERY`, and `CREATE` for a missing object. A TTL difference is deferred: the operator command applies it without materializing it, so it takes effect as parts merge. The diff reports and refuses anything else it detects, a type change or a different sorting key, partition key or engine family, because each is a mutation or a rebuild.

The reconciler leaves anything present in the live table but absent from the desired schema alone, because a diff cannot distinguish a drop from a rename, and either interpretation loses data. The plan contains a list of statements. With `create_schema: true`, the backend applies the additive statements at startup. With `create_schema: false`, nothing runs at startup, as today. A command prints and applies the plan on demand.

This approach has two properties that no versioned design provides. The templates remain the single readable source of truth, so a contributor's PR contains only the template edit. There is no number to assign or file to freeze. The reconciler also moves a drifted database toward the desired state instead of assuming that it matches a version number. Such drift comes from an operator who added an index by hand, or from an earlier version of the binary that created a table slightly differently.

The same engine produces a versioned audit trail instead of consuming one. At release time, it compares the previous release's rendered schema with the current schema through scratch tables. The result is a numbered migration file to commit for people who want one.

#### Third-party reconcilers for ClickHouse

The engine and its verification step are several hundred lines of code, so a search for an existing library came first. It found none that is both embeddable in a Go binary and under a free license.

| Tool | Desired-state input | Diff method | License | Embeddable |
| --- | --- | --- | --- | --- |
| Atlas | HCL or SQL | live introspection, plan | Apache 2.0 core; the ClickHouse driver is in the paid Pro tier | ❌ |
| PostHog/chschema | HCL | live introspection, `diff -sql`; flags engine, `ORDER BY` and `PARTITION BY` changes as unsafe | MIT | ❌ Go, but all code under `internal/`, CLI only |
| Noibu/housekeeper | SQL DDL file | generates `ALTER`s, `ON CLUSTER` | GPL-3.0 | ❌ license |
| anytoe/chsync | live or `.sql` snapshot | catalog diff | MIT | ❌ Go under `internal/`; no indexes, codecs or TTL; self-described as not battle-tested |
| zifter/clickhouse-migrations 0.14 | `schema.sql` | scratch database plus catalog diff; refuses engine, `ORDER BY`, `PARTITION BY`, `SETTINGS`; comments out drops unless `--allow-destructive`; `KeeperMap` lock | MIT | ❌ Python |
| Liquibase with the ClickHouse extension | changelog | none for ClickHouse (`diffChangeLog` is not supported by the extension) | Apache 2.0 | ❌ Java, imperative |

Two tools provide relevant prior art, although this RFC adopts none of the six. The Python tool compares throwaway objects through the catalogs, as §5 proposes, and refuses the same class of changes. This independently supports the method's use on ClickHouse. The tool also implements the `KeeperMap` lock described in §2.4. The unsafe-change classification in `chschema` matches §2.3.

### 4.6 Comparison

| Criterion (§3) | 4.1 golang-migrate deltas | 4.2 in-house deltas | 4.3 full copies | 4.4 living templates + deltas | **4.5 reconciliation** |
| --- | --- | --- | --- | --- | --- |
| 1. Per-object deltas | 🟢¹ | 🟢 | ❌ | 🟢 | 🟢 |
| 2. One source of truth | 🟢 | 🟢 | ❌¹⁹ | 🟨² | 🟢 |
| 3. Template parameters survive | 🟢³ | 🟢 | 🟢 | 🟢 | 🟢 |
| 4. Database state knowable | 🟨⁴ | 🟨⁴ | ❌ | 🟨⁴ | 🟢⁵ |
| 5. Safe with concurrent instances | ❌⁶ | 🟨⁷ | n/a | 🟢⁸ | 🟢⁸ |
| 6. Operator-managed schema | 🟨⁹ | 🟨⁹ | 🟢 | 🟨⁹ | 🟢¹⁰ |
| 7. Readable current schema | 🟨¹¹ | 🟨¹¹ | 🟢 | 🟢 | 🟢 |
| 8. Dependency footprint | 🟨¹² | 🟢 | 🟢 | 🟢 | 🟢 |
| 9. Little bookkeeping | ❌ | ❌ | 🟨 | 🟨¹⁸ | 🟢¹³ |
| 10. Production statements reviewable | 🟢 | 🟢 | 🟨 | 🟢 | 🟨¹⁴ |
| 11. Operator customizations survive | 🟨¹⁵ | 🟨¹⁵ | 🟨¹⁵ | 🟨¹⁵ | 🟨¹⁶ |
| 12. Planning writes nothing | 🟢 | 🟢 | n/a | 🟢 | ❌¹⁷ |
| Code to own | 🟨 glue plus the driver workarounds of §4.1 | 🟨 ~100 lines | 🟢 | ❌ the §5.2 diff for generation and parity, plus a runner and a ledger | ❌ the §5.2 diff plus a verification step |

- ¹ The library reads each version as one stream, so the backend can concatenate per-object files in memory. The single baseline file in #9796 is that PR's choice, rather than a framework constraint.
- ² Templates and deltas describe the schema separately. Only the CI parity check in §4.4 keeps them equal. That check runs the §5.2 diff against two databases.
- ³ As it stands, #9796 renders the baseline through the templates; the earlier shape that its review commented on hard-coded the engine.
- ⁴ A version number states what the ledger records, rather than what the tables contain. It does not reveal drift.
- ⁵ The live catalog determines the plan, so the report describes what the database actually lacks.
- ⁶ The library does not create its version table idempotently, so first starts race. Pending versions can also interleave across instances.
- ⁷ This is correct if every statement is idempotent and no version reassigns a property assigned by an earlier version. A `KeeperMap` lock would make it 🟢.
- ⁸ Startup applies only additive statements. They are idempotent and commute up to a difference reported by the next plan (§5.5), so they need no coordination. Only a one-shot operator command runs non-additive statements. Option 4.4 shares this property in its strongest form. Its version record adds one write, made idempotent by inserting a row per version instead of updating one row.
- ⁹ The operator receives the files and the message "versions 2 and 3 are pending". The mechanism trusts the operator to apply the files in full.
- ¹⁰ The operator receives exactly the statements that the database lacks, computed against that database.
- ¹¹ A generated golden dump provides the readable current schema, rather than the authored files.
- ¹² The library's driver requires a second `database/sql` connection.
- ¹³ A contributor edits only the template. The engine generates the release snapshots in §5.7; contributors do not author them. The engine reads database state rather than recording it.
- ¹⁴ The PR shows the template edit rather than the `ALTER` statements. The release-time migration file (§5.7) and printed plan show those statements.
- ¹⁵ A delta touches only the properties it names. However, a `MODIFY COLUMN ... CODEC` or `MODIFY SETTING` delta overwrites an operator's customization of that property just as a computed statement does.
- ¹⁶ The reconciler never removes a table, view, column or index absent from the template. Each plan reports a customized codec, setting, default or TTL as a deferred difference, and `apply` overwrites it. Of these properties, only the TTL's removal changes data volume, because a TTL that an operator set by hand under `ttl: 0` stops expiring rows once `apply` removes it. An operator who customizes one therefore keeps it by not running `apply`, or by carrying the customization into the template.
- ¹⁷ A plan creates and drops scratch tables in the configured database (§5.2). This requires `CREATE TABLE`, `DROP TABLE` and `SHOW COLUMNS` there, plus `SELECT` on one system catalog (§5.8). It never touches a live Jaeger object. Temporary tables would avoid these writes but omit properties needed for the diff (§2.4).
- ¹⁸ The diff engine generates the delta, and contributors review it rather than write it. The version number, ordering rule, frozen-file rule and ledger upkeep from §4.2 remain.
- ¹⁹ The full schema copy and the human-written migration script from the previous copy are two descriptions of the change. Nothing checks them against each other.

This RFC recommends reconciliation and weighs it against option 4.4, because the two share the diff engine, the additive-only startup and the operator command, and differ in one thing: where production statements come from. Reconciliation computes them against the live catalog. Option 4.4 uses statements computed by the same engine at PR time against the previous release and read by a reviewer.

Rows 4, 6, 7 and 9 decide the recommendation. The live database determines the plan, and the operator receives exactly the statements that database lacks. The templates remain the readable schema. Contributors have no numbering or freezing rules to learn, so a fleet deployed from trunk has no ordering problem to solve. Reconciliation loses row 12, because scratch tables need writes and two grants. It also loses the code-to-own row, catalog comparison plus a verification step, although the third-party tools of §4.5 already solve that shape. Release-time migration files and the printed plan recover row 10.

The objection to computing statements in production is that a live difference has no label. The catalog cannot distinguish an operator's customization, a partially completed upgrade and a schema created by the previous release. A delta that names its change needs no such distinction. This design answers the objection by never interpreting a difference. Each compared property maps to one fixed action in the table of §5.2: the reconciler applies additive statements, reports deferred ones and refuses the rest. It never removes a live object. It therefore reports all three cases in the same words and leaves their resolution to the same operator. Under option 4.4, the operator would not learn of those cases at all.

What remains of the objection is real: the diff engine is the production path, so a defect in it is a production defect rather than a CI failure. Three safeguards mitigate this risk. The compared properties form a closed list, tested row by row (§5.9). Startup executes only the three additive statement kinds. Option 4.4 remains the fallback if the engine proves harder than estimated, since it needs the same engine for its generation and parity steps, shares the per-object layout and the idempotence rules, and adds the version record.

---

## 5. Design

### 5.1 Desired state

The twelve files keep their locations and names. They remain the complete description of the desired schema and change in place when the schema changes. The OTLP columns in #9789 add ten lines to `create_spans_table.sql`, and RFC 0017's codecs change that same file. The templates render with the `schemaTemplateParams` type as today.

### 5.2 The reconciler

```go
// schemaReconciler computes and applies the statements that bring the live schema
// to the rendered templates.
type schemaReconciler struct {
    conn   driver.Conn
    params schemaTemplateParams
}

// plan renders the desired tables under scratch names, reads both scratch and
// live objects from the system catalogs, and returns the statements that would make
// the live objects match, in template order so that a table is created or altered
// before the views that read it.
func (r *schemaReconciler) plan(ctx context.Context) (schemaPlan, error)

// apply executes the statements of a plan in order. Refusals are reported by the
// caller; they never gate the statements the plan can execute.
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

The `plan()` method uses the factory's existing native protocol but opens its own connection. It performs the following steps:

1. The method opens its own client with one connection to a single address, because the factory's `driver.Conn` is a pool over every configured address, and scratch objects created through one transport are not visible through another server's transport. The method tries configured addresses in turn until one answers and accepts the first scratch `CREATE`. This is necessary because a replica of a `Replicated` database refuses to enqueue DDL while it catches up if its log pointer trails the others by more than `max_replication_lag_to_enqueue`. The method renders each table template with `TablePrefix` set to `_jaeger_scratch_<random>_` and the local engine family, regardless of `table_engine`. A plan therefore needs no `table_engine` setting to compute. The method creates the scratch tables in the configured database with a `COMMENT` containing a fixed marker and the run's identity. That comment is what marks a table as a scratch table. The templates gain `{{ .TablePrefix }}` before each table name; it renders empty for the live schema. Scratch tables require `CREATE TABLE` and `DROP TABLE` on the configured database. Today's `create_schema: true` requires `CREATE TABLE`, plus the database `SELECT` and `INSERT` grants that `CREATE MATERIALIZED VIEW ... TO` requires on its target. The `DROP TABLE` grant is therefore new, and §6 lists it. In a `Replicated` database, scratch DDL replicates like all other DDL and costs twelve log entries per start (step 4). Such a database may be configured to accept only `Replicated*` engines, so scratch tables there use the argument-less `Replicated*` engine that the database completes. The method creates no scratch views. Instead, the server's `formatQuerySingleLine` function normalizes each view's query by reprinting the template's `SELECT` in the form stored by `as_select`.
2. For each desired table, the method reads `system.tables`, `system.columns` and `system.data_skipping_indices` for both the scratch and live copies. A scratch table that reads back no columns fails the plan. The reconciler itself just created that table, so an empty read means that `system.columns` is hiding its columns from a user who lacks `SHOW COLUMNS`, and without this check the empty read would pass for an empty diff. For each view, the method reads the live view's `system.tables.as_select` and `create_table_query` values. ClickHouse qualifies table references with the database at `CREATE` time. The comparison therefore strips the configured database's qualifier from the live `as_select` and the scratch prefix from the scratch side. Both sides flatten a first-level `Nested` group to one `Array` column per member. A group nested inside another group, such as the attribute groups under `events` and `links`, remains one `Array(Nested(...))` column. A change inside it is therefore a type change.
3. The method computes the difference per object. A missing live object yields its rendered `CREATE` with the deployment's engine. A missing column yields `ADD COLUMN IF NOT EXISTS`, placed `AFTER` its predecessor. A missing index yields `ADD INDEX IF NOT EXISTS`. An index with a different `type_full`, `expr` or `granularity` yields `DROP INDEX IF EXISTS` followed by `ADD INDEX`. A codec difference yields `MODIFY COLUMN ... CODEC`, or `MODIFY COLUMN ... REMOVE CODEC` if the desired column has none. A setting difference yields `MODIFY SETTING`. A TTL difference yields `MODIFY TTL ... SETTINGS materialize_ttl_after_modify = 0`, or `REMOVE TTL` if the desired schema has none. A default expression difference yields `MODIFY COLUMN ... DEFAULT`. A different view `as_select` yields `MODIFY QUERY` with the query text from the rendered template. The table under "What is compared" below lists every compared property and its result. A difference with no statement yields a refusal. Live columns and indexes absent from the desired schema yield nothing. The method applies the statements in two passes, every table's first and then every view's, and runs the comparison again between and after the passes against the same scratch tables, for up to three rounds. The reason is that two releases that both find a table missing both plan only its `CREATE`, and the one whose `CREATE` lost still has columns to add before its views can select them.
4. The method drops the run's scratch tables after every comparison has read them, including the verification in §5.6. It also drops them on error. A start therefore writes twelve scratch `CREATE` and `DROP` statements and no more. In a `Replicated` database, each statement adds a DDL log entry replayed on every replica. These entries count against the database's `logs_to_keep` setting, whose default is 1000 entries. Beyond that limit, the database treats a lagging replica as lost and re-synchronizes its metadata, so the per-start count matters. A crashed run leaves scratch tables behind. The next run drops tables that carry the marker comment and have a `metadata_modification_time` more than an hour old. A table with the prefix but no marker belongs to someone else and is never touched. If a run finds one of its own scratch tables missing after an hour-long pause allowed the sweep to remove it, the plan fails with an error and startup is retried. The method never interprets that absence as a missing object. The sweep can therefore cause at most one failed start in a case that requires a pause of over an hour. This provides the ownership guarantee that the prefix alone cannot provide.

#### What is compared

The catalog columns below define the comparison for each object kind. The reconciler compares no property absent from this table. An empty plan therefore certifies exactly these properties and nothing else.

| Object | Property | Source | On difference |
| --- | --- | --- | --- |
| table | exists | `system.tables` | `CREATE` with the engine `table_engine` renders (additive); a refusal naming the setting when the configuration has none, because the reconciler never guesses an engine |
| table | engine family (`MergeTree`, `AggregatingMergeTree`) after stripping `Replicated` or `Shared` | `system.tables.engine` | refusal |
| table | replication mode against `table_engine`, when the configuration sets it: a local engine under `replicated`, or a `Replicated*` engine under `merge_tree`, is a mismatch; `Shared*` satisfies either, because ClickHouse Cloud substitutes it for both | `system.tables.engine` | refusal |
| table | sorting key, partition key, primary key, sampling key | `system.tables` | refusal |
| table | TTL clause, the text of `engine_full` between `TTL` and `SETTINGS`, which the server prints in a fixed order on both sides | `system.tables` | `MODIFY TTL` or `REMOVE TTL` (deferred) |
| table | constraints and projections | none declared by a template today; a template that declares one is a refusal until the comparison learns the property | refusal |
| table | settings, the `key = value` list of `engine_full` after `SETTINGS` | `system.tables` | `MODIFY SETTING` (deferred); the settings ClickHouse rejects with `READONLY_SETTING`, `index_granularity` and `index_granularity_bytes` among them, are a refusal, from a list kept in the code because the catalog does not mark them |
| column | exists | `system.columns` | `ADD COLUMN IF NOT EXISTS ... AFTER` (additive) |
| column | type | `system.columns.type` | refusal |
| column | default expression, for the plain `DEFAULT` kind | `system.columns` | `MODIFY COLUMN ... DEFAULT`, or `REMOVE DEFAULT` when the template has none (deferred, and subject to rule 4 of §5.4) |
| column | default kind | `system.columns.default_kind` | none and `DEFAULT` convert into each other through the row above; any transition involving `MATERIALIZED`, `ALIAS` or `EPHEMERAL` is a refusal |
| column | codec | `system.columns.compression_codec` | `MODIFY COLUMN ... CODEC`, or `MODIFY COLUMN <name> REMOVE CODEC`, which takes no type, when the template has none (deferred) |
| index | exists | `system.data_skipping_indices` | `ADD INDEX IF NOT EXISTS` (additive) |
| index | `type_full`, `expr`, `granularity` | `system.data_skipping_indices` | `DROP INDEX` then `ADD INDEX` (deferred) |
| view | exists | `system.tables` | `CREATE MATERIALIZED VIEW IF NOT EXISTS` (additive; a view carries no engine, so it needs no `table_engine`) |
| view | `TO` table | `create_table_query` | refusal |
| view | query, the template's `SELECT` through `formatQuerySingleLine` against the live `as_select` | `system.tables.as_select` | `MODIFY QUERY` with the template's text (deferred) |

The comparison deliberately excludes the Keeper path and replica name of a `Replicated*` engine. Deployments can legitimately use paths that no template reproduces. It also excludes a view's declared columns, which follow from the compared query; the position of existing columns; and comments. Any live column, index, projection or constraint absent from the template is also excluded. An operator may have added it, and the reconciler never removes it.

The method strips database qualifiers only when they name the configured database. A deliberate cross-database reference therefore survives. Both sides flatten `Nested` groups in the same way. A group nested inside another remains one `Array(Nested(...))` column, so a change inside it produces a type difference.

The reconciler never changes an engine because ClickHouse cannot change it in place (§2.3). The two engine rows detect a configuration that disagrees with the tables, such as `table_engine.replicated` over local tables or `merge_tree` over replicated tables. The `SharedMergeTree` engine counts as replicated because ClickHouse Cloud stores every `MergeTree` table that way.

#### Additive and deferred statements

Each statement belongs to one of two classes. Its class determines who runs it.

- **Additive**: `CREATE ... IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `ADD INDEX IF NOT EXISTS`. Each statement is idempotent. Two such statements from different releases for the same object either agree or preserve the definition that ran first. The next `plan` reports the other definition as a deferred difference. If the releases disagree on a type, it reports a refusal instead; rule 2 of §5.4 forbids a release from creating that disagreement. Nothing is lost or silently certified. Additive statements therefore require no coordination, and startup applies them.
- **Deferred**: `MODIFY COLUMN`, `MODIFY SETTING`, `MODIFY TTL`, `REMOVE TTL`, `DROP INDEX` plus `ADD INDEX`, `MODIFY QUERY`. These statements do not commute. If an older release's `MODIFY QUERY` runs after a newer release's statement, it computes derived rows with the wrong query until someone notices. An older TTL can expire parts in that interval. ClickHouse has no primitive that safely orders these statements across concurrent instances. Distributed DDL keeps executing after its lease expires, a `Replicated` database retries remote DDL until completion, and a lock cannot revoke queued work. Startup therefore never applies deferred statements by default. It reports them, and the one-shot operator command in §5.8 applies them from one place at a time. The opt-in `apply_deferred_changes: true` also allows startup to apply them. A deployment sets this only if it ensures by construction that a single instance reconciles, as in a single-node installation. It also accepts that rollback applies the deferred statements backwards. The previous release's templates differ from the schema applied by the newer release, so its startup applies that difference too. A per-pod init container does not ensure a single reconciler: a rolling deployment runs old and new releases' init containers concurrently.

### 5.3 Startup behavior

| `create_schema` | Plan | Behavior |
| --- | --- | --- |
| `true` | empty | Start. |
| `true` | additive statements, possibly with deferred ones and refusals | Apply the additive statements, verify them (§5.6), log the deferred statements and the refusals with the operator command that applies or shows them, and start. With `apply_deferred_changes: true`, apply the deferred ones too. |
| `true` | only deferred statements and refusals | Log them and start; with `apply_deferred_changes: true`, apply the deferred ones, verify them, and start. |
| `true` | cannot be computed | Fail: creating the schema was asked for and needs the privileges to plan it. |
| `false` | not computed | Start, as today. Nothing is created, read or dropped at startup; `clickhouse-schema plan` computes the plan on demand, with whatever user has the privileges. |

Startup fails for exactly three reasons: the plan cannot be computed, an additive statement fails to apply, or the verification in §5.6 finds an additive statement still outstanding. Failure to apply includes a Jaeger user without the `ALTER` grant. The last two failures mean that a column written by this binary is missing somewhere. That is the one difference that prevents the binary from working.

A refusal means that the reconciler cannot resolve a difference. It does not establish that the binary cannot run. A binary whose template specifies `String` can still write a column changed to `LowCardinality(String)` by an operator's runbook. Likewise, the previous release restarted during a rollback sees differences from a schema expanded by the newer release. Failing either start would prevent reversal after every runbook. Refusals therefore appear at every start but block nothing. One refusal does report a discrepancy that affects the binary: a table engine that disagrees with `table_engine`. This is a configuration error, and the log tells the operator to correct the configuration.

A deployment on the previous release needs no bootstrap step. The first start with this mechanism computes a plan against its tables in the same way as any other start.

### 5.4 Rules for changing the schema

1. **Expand only.** A template edit adds columns, indexes, codecs and settings or changes a view's query. It never removes or renames a column read or written by the previous release. A removal changes the template in the release after the one that stops using the column. Even then, the reconciler does not drop the column; an operator runbook does. During a rolling deploy, an older binary therefore always runs against a schema that is a superset of its expectations. The existing `INSERT` statements with explicit column lists and `SELECT` statements with explicit columns already tolerate that schema.
2. **No data-rewriting changes at startup.** Operator procedures with runbooks change column types, sorting keys, partition keys and engines. Column type changes include `LowCardinality` and any change inside the nested attribute groups of `events` and `links`. A TTL change is deferred and runs without materialization (§2.3). A template edit that changes one of these properties is legitimate for fresh installs. Against existing databases, the reconciler reports a refusal at every start until the runbook runs. The PR that changes the template therefore ships the runbook. The test in §5.9 that reconciles from every prior release fails until the PR includes it. The runbook has to leave the previous release able to write the table, because that release restarts during a rollback and the refusal does not stop it (§5.3). Widening a type to `LowCardinality` or `Nullable` meets this requirement; narrowing a type does not.
3. **A view change is forward-only and must leave old rows valid.** `MODIFY QUERY` and `CREATE` of a missing view affect only rows inserted afterwards. They repair no rows already in the target table. A view edit may therefore add derived columns, for which old rows hold defaults. It may also change the query if existing target rows remain correct under the new definition. A change to grouping, filtering or aggregation that makes old rows wrong requires more than a template edit. It ships with a backfill runbook that repopulates the target from `spans`, and the PR states that requirement. Because `MODIFY QUERY` is deferred, successful startup does not enable a feature that depends on the changed view. The release notes state that `clickhouse-schema apply` is a prerequisite. A view is never dropped and recreated, because inserts during the gap would bypass it.
4. **Defaults are semantics, not metadata.** A new column takes its type default. An `INSERT` from a binary that does not know about the column therefore still succeeds. Changing an existing column's `DEFAULT` requires only metadata changes to execute, but it changes the data read. Parts written before the column existed hold no value for it and evaluate the current default. After a default change, a row in such a part reads the new default. A row in a part that materialized the column retains the old value. Thus, a row's value depends on its part's history. A default change is acceptable only if every such row is correct under the new default. Otherwise, it ships with a runbook that runs `ALTER TABLE ... MATERIALIZE COLUMN` before the change. The deferred `MODIFY COLUMN ... DEFAULT` in §5.2 executes the change; it does not certify its correctness.

### 5.5 Concurrency

Startup applies only additive statements (§5.2), each idempotent, in template order. Concurrent instances of one release produce the same schema. A duplicate `ADD COLUMN IF NOT EXISTS` normally does nothing. However, on a `Replicated*` table reached concurrently through two replicas, it can fail with `CANNOT_ASSIGN_ALTER` (error 517, "metadata on replica is not up to date"). The server's message states that the error can be retried. The losing statement did not execute. Although the replication log brings the winner's entry to the losing replica, the losing instance has to see its own statement applied before verification. Each additive statement therefore retries that error with backoff for up to a minute. If retries expire, startup fails, and the next start plans again.

Concurrent instances of two releases produce the union of their additive statements. If they define a new column or index differently, the first definition remains. Every later `plan` reports the second definition as a deferred difference for a codec, default or index definition, and as a refusal for a type. The disagreement therefore remains visible instead of being resolved by chance. Additive statements commute up to that visible difference, which is exactly the claim in footnote 8 of §4.6 and no more. These rules provide the entire startup concurrency guarantee without a lock, version table or clock.

The same rules bound the work charged to a fleet. Every startup statement changes only metadata, the scratch tables are empty, and retries have a time limit. A readiness probe therefore sees seconds of schema work rather than a mutation. The exception is an additive statement that arrives while an operator's mutation runs on the same table (§5.6). A fleet that wants no schema work in its pods runs `clickhouse-schema apply` as one pre-upgrade job (§5.8) and sets `create_schema: false` in the pods. This is also the least-privilege configuration, because the pods then need only `INSERT` and `SELECT`.

This RFC designs the operator command for deferred statements to run once, from one place. As a guard rail against two operators running it at once, the command takes a lock on a server that offers `KeeperMap` (§2.4). A `jaeger_schema_lock` table holds one row keyed by the database name. The value contains the holder's identity and a lease expiry. Every mutation uses `keeper_map_strict_mode = 1` and a predicate on the current value. Acquisition, renewal, takeover of an expired lease and release are therefore compare-and-set operations that fail instead of overwriting another value.

The lock row is created once and never deleted. Release uses the same conditional update to write an empty holder and an expired lease. A later acquisition takes the free row through that operation too. This is necessary because `KeeperMap` compares a row's Keeper version rather than its content. A deleted and recreated row starts again at version 0. A delete from a paused former holder could therefore remove a row just recreated by a later holder.

A mutation whose predicate matches no row succeeds without changing anything. The command therefore reads the row back after every mutation and holds the lease only while the row names its identity. If the read returns another holder, the mutation did not take effect, and the command stops. The lock serializes the command's own statements but cannot revoke distributed DDL queued by a previous holder. It is therefore a guard rail; the instruction to run the command from one place provides the guarantee. If `KeeperMap` is unavailable, the command reports that fact and runs unlocked.

### 5.6 Replicated clusters

The reconciler computes the plan against the replica that the connection reached. Column, index, codec and TTL `ALTER` statements on `Replicated*` tables propagate through Keeper. `CREATE`, `MODIFY SETTING` and a view's `MODIFY QUERY` affect only that replica (§2.3). A deployment with more than one replica therefore needs DDL fan-out.

The alternative would repair each replica when an instance next connects to it. That works only while the replica's Keeper metadata matches the current template. A template `CREATE` against metadata with a different column or codec fails with `INCOMPATIBLE_COLUMNS`. This includes differences added by an operator, still deferred, or present only in the template. A different TTL or skip index causes `METADATA_MISMATCH`. Only a settings difference is accepted, because it is local to the replica. Repair through whichever instance connects next is therefore not reliable. ClickHouse offers two mutually exclusive fan-out mechanisms.

- **A `Replicated` database**, or the `Shared` database that ClickHouse Cloud assigns to every database, replays every DDL statement on every replica. The `Shared` engine replicates DDL in the same way. The reconciler detects either engine through `system.databases.engine`. It issues DDL with `distributed_ddl_output_mode = 'throw_only_active'`, which waits for replicas that are up and reports a failure on any of them. In contrast, `throw` waits the full `distributed_ddl_task_timeout` for a down replica. Such a database rejects `ON CLUSTER` with any name except its own. Since ClickHouse 24.9, it also rejects `Replicated*` engines with explicit arguments by default, through `database_replicated_allow_replicated_engine_arguments`. This deployment therefore uses `table_engine.replicated` without `keeper_path`, `replica_name` or `cluster`; the database supplies the replication arguments. A `Replicated` database registers a cluster under its own name, and the verification of this section reads through that cluster; on ClickHouse Cloud the `default` cluster takes that role instead. That cluster connects with the credentials of the named collection in the database's `collection_name` setting, or as `default` with no password when there is none. A self-hosted `Replicated` database with a password on its `default` user therefore needs a collection with `cluster_username` and `cluster_password` before verification can read other replicas. #9813 made the `keeper_path` and `replica_name` fields optional so that the server's `default_replica_path` and `default_replica_name` apply when they are absent, which is the same rendering.
- **`ON CLUSTER`** provides fan-out for a self-hosted cluster with an ordinary `Atomic` database. The backend adds an optional `cluster` field to `table_engine.replicated`. When set, a template parameter adds `ON CLUSTER <name>` to every DDL statement that the reconciler issues against the live database. Those statements use the same `distributed_ddl_output_mode = 'throw_only_active'` as above, because the default mode throws `TIMEOUT_EXCEEDED` after `distributed_ddl_task_timeout` while a host is down, even though the task stays queued for it. For `ON CLUSTER` tasks, the DDL worker publishes the active markers used by `throw_only_active` on every release admitted by the floor in §8. The lock statements in §5.5 never carry this clause, because ClickHouse rejects a mutation with `ON CLUSTER` on a `KeeperMap` table. The reconciler reads catalogs through `clusterAllReplicas(<name>, system.tables)` and its siblings, always with `skip_unavailable_shards = 1`. The default fails the entire read if one host is down. Reading all available replicas ensures that a replica-local setting or view query that has drifted elsewhere still produces a statement. The `clusterAllReplicas` function connects to other hosts with the credentials in `remote_servers`, or as the initiating user if the cluster defines a `cluster_secret`. The ClickHouse documentation says so, and it matters because a cluster with neither connects as the `default` user with no password and fails the catalog read wherever that user has a password or is disabled. The Jaeger user needs the `CLUSTER` grant for `ON CLUSTER` statements, as required by the shipped server's `on_cluster_queries_require_cluster_grant` setting. It also needs `REMOTE ON *.*` for `clusterAllReplicas` under either fan-out mechanism. This design treats `cluster` on a `Replicated` database as a configuration error reported by `plan`, even though the server does not impose that rule. A deployment therefore never carries both fan-out mechanisms.

The design assumes the single-shard layout that the backend writes to today. The reconciler sets `alter_sync = 1` explicitly on every `ALTER`, because ClickHouse Cloud ships `0` as its default on several releases; the second `plan` below is what certifies the replicas. Under either fan-out mechanism, the DDL worker executes an `ALTER` on a `Replicated*` table on one replica per shard. Other replicas apply the replication-log entry afterwards. The statement can therefore return before the replica connected to Jaeger has the column.

The other wait modes do not provide a suitable guarantee. With `alter_sync = 2`, an active replica can delay completion without a bound. A down replica throws `UNFINISHED` after `replication_wait_for_inactive_replica_timeout`. Within `ON CLUSTER`, the DDL worker treats that error as retriable and re-executes the statement up to three times. The `alter_sync = 3` mode waits only for active replicas, but the settings catalogs of some supported releases do not describe it. The design therefore relies on neither mode.

A down replica applies the log entry when it returns. No insert reaches it while it is down. However, it accepts connections before its replication queue reaches the entry, which follows the fetches queued during the outage. An insert routed there in that window fails with `NO_SUCH_COLUMN_IN_TABLE`. This window lasts for the outage backlog and is the one gap in tolerance of down replicas. A fleet that cannot accept the gap runs `clickhouse-schema apply` as the pre-upgrade job in §5.8 while every replica is up. That job verifies all replicas.

One wait is inherent in the design. A `Replicated*` table executes alters in order. An additive statement that arrives during a type-change mutation (§5.4) on that table waits for the mutation to finish. For `spans`, this takes hours. The runbook for such a change therefore runs `clickhouse-schema apply` first, leaving no additive statement pending when the mutation starts. It also states that a pod restarted with a pending additive statement during the mutation waits for it.

After the additive statements and before the backend serves traffic, the `plan()` method runs again through `clusterAllReplicas`. It runs over `cluster` when that is set, and otherwise over the cluster that a `Replicated` database registers under its own name, or `default` on ClickHouse Cloud. This is why the `REMOTE` grant in the second bullet applies to both mechanisms. Replicas apply log entries asynchronously, so verification repeats with backoff for up to a minute while an additive statement remains outstanding on a replica that answers. If it remains outstanding after that minute, startup with `create_schema: true` fails, because a replica that still lacks a column would fail the first insert routed to it. The next start plans again.

A down replica is neither certified nor fatal. The second plan logs deferred differences without failing startup. This RFC declares a multi-replica deployment with neither fan-out mechanism unsupported for `create_schema: true`, for the reason above, and the backend's documentation says so.

### 5.7 Release snapshots and migration files

The diff engine also runs offline against two sets of rendered templates. It creates both sets as scratch tables in a CI ClickHouse instead of comparing with a live database. The `make clickhouse-schema-snapshot` target writes the rendered schema with default parameters to `internal/storage/v2/clickhouse/sql/snapshots/<release>.sql`. It also writes the plan from the previous snapshot to the new one to `migrations/<previous>_to_<release>.sql`. Refusals appear as comments that name the runbook. The release PR commits both files.

These files provide documentation and an audit trail. Their default parameters make them unsuitable for execution against a deployment with its own `ttl`, `table_engine` or `cluster`. The binary never reads them, and each file's header states this restriction. Operators obtain executable statements from `clickhouse-schema plan`, which renders them with the deployment's configuration.

### 5.8 Operator path

```
jaeger ... --config config.yaml clickhouse-schema plan      # the plan against the live database, apply nothing
jaeger ... --config config.yaml clickhouse-schema apply     # apply the whole plan, additive and deferred, once
jaeger ... --config config.yaml clickhouse-schema render    # the rendered desired schema
```

The `plan` command prints the plan and exits non-zero when it is non-empty. It can therefore serve as a CI check for a database fully described by the templates. It prints refusals with the statements an operator would run. An operator with a standing customization (§6) reads the output rather than relying on the exit code.

The command needs `CREATE TABLE`, `DROP TABLE` and `SHOW COLUMNS` on the configured database. The last grant is necessary because `system.columns` silently hides every column of a table that the user cannot see. Without it, the column diff could be empty on both sides and produce an empty plan. The command also needs `SELECT ON system.data_skipping_indices`, which the implicit system grants omit under the shipped `select_from_system_db_requires_grant` setting. A multi-replica cluster needs `REMOTE ON *.*`, plus `CLUSTER` when `cluster` is set (§5.6).

The `table_engine` setting is needed only to render `CREATE` for a missing table. Without it, all comparisons of existing objects run, except the replication-mode row. A missing table produces a refusal that names the setting. This is the normal operator-managed configuration today, and the tests cover it (§5.9).

The `apply` command runs the one-shot procedure in §5.5. In addition to the plan grants, it needs `ALTER` on the database's tables and `ALTER VIEW MODIFY QUERY` on its views. It also needs `SELECT` and `INSERT` on the database. `CREATE MATERIALIZED VIEW ... TO` requires both on its target, and the lock table in §5.5 uses them for reads and writes. The command takes a lock where the server offers one, applies additive and deferred statements in template order, and verifies the result. It prints refusals it cannot resolve and exits non-zero if any remain.

The `render` command requires `table_engine` and says so when the setting is absent, because a rendered schema without engines is not one. The `apply` command is also the form for fleets that want the schema settled before pods roll: one job that runs before the rollout, such as a Helm pre-upgrade hook, and not a per-pod init container, which a rolling deployment runs once per pod and therefore concurrently across two releases.

All three commands render with `ttl`, `table_engine` and `cluster` from the configuration file. Their output therefore matches the deployment rather than a default. The collector owns the root `jaeger` command, which registers `--config` as a local flag. Cobra does not pass that flag to subcommands, so the subcommand declares the same `--config` flag and feeds it to the same configuration loader.

### 5.9 Tests

All tests run against the live ClickHouse already started by the integration suite.

1. **Fresh install.** The test starts the backend on an empty database and checks that `plan` is empty afterwards. Every compared property (§5.2) of every object must equal the rendered template after the server normalizes it.
2. **Upgrade from every release.** For each snapshot in `snapshots/`, the test applies the snapshot to an empty database and writes the integration corpus through that release's writer. It then starts the backend and runs `clickhouse-schema apply`. The compared properties must equal those of a fresh install, and the resulting plan must be empty. The current reader must also read the corpus back, because catalog parity alone does not show that rows written before the upgrade remain valid under the new defaults and view queries. A template edit that needs a runbook fails this test with a refusal until the runbook exists and the test lists the change as expected.
3. **Idempotence and concurrency.** The test starts the backend twice and also starts it concurrently from two connections. The plan must be empty after each case. It then starts two binaries concurrently whose templates define a new column with different codecs. The column must exist with one codec, and the next `plan` must report the other as a deferred difference. The test repeats this for an index definition and a view query. It also repeats it for a type, for which the next `plan` must report a refusal. Another case starts two binaries against an empty database, with one defining an extra column and a view that selects it. Both columns and the view must exist after both binaries start. Against a two-replica table, the test runs `ADD COLUMN IF NOT EXISTS` concurrently through both replicas. Both starts must succeed through the retry on error 517.
4. **Refusals.** For each refused row in the §5.2 table, the test creates an object that differs from the template in that property alone. The `plan` command must return the refusal and exit non-zero. Startup with `create_schema: true` must log it and succeed. For a widened type, the writer must insert successfully afterwards. Cases include a local table under `table_engine.replicated` and a replicated table under `merge_tree`. Another case runs `plan` with the default `create_schema: false` configuration and no `table_engine` against a database missing one table. This must yield the refusal naming the setting while all other comparisons run. Every `plan` and startup assertion uses a user with exactly the grants listed in §5.8, never `default`. A grant omitted from that list therefore fails the test rather than the operator's deployment.
5. **Deferred statements.** The test covers each deferred row in the §5.2 table, including removal of a codec or default from the template. Startup must preserve and log the difference. The `clickhouse-schema apply` command must resolve it, and `apply_deferred_changes: true` must resolve it at startup. With `create_schema: false`, startup must issue no DDL at all. The test checks this through `system.query_log`.
6. **Diff engine unit tests** use synthetic catalog rows. They cover every row in the §5.2 table, the ignored properties, `Nested` flattening, column placement, cross-object ordering, TTL clause extraction, and prefix and qualifier stripping. They also cover `formatQuerySingleLine` normalization of every view template.
7. **Lock and replication.** The lock tests run against ClickHouse with its embedded Keeper, which a single server supports. Two concurrent `clickhouse-schema apply` runs must leave one waiting. An expired lease must be taken over exactly once. A renewal racing a takeover must lose, and a release racing a successor must preserve the successor's row. Without `KeeperMap`, the command must report that fact and run unlocked. A two-replica Compose job covers convergence through `ON CLUSTER` and through a `Replicated` database. It also checks that `cluster` set on a `Replicated` database is reported. A replica down during startup must leave startup successful and receive the column when it returns. Verification must fail startup when a replica that answers still lacks a column.

The tests run once with `merge_tree` and `ttl: 0`, and once with `ttl` set, to cover the TTL clause against a live server. They run against both the minimum ClickHouse version in §8 and the latest release, because server versions can change the catalog columns read by the diff. The CI ClickHouse already runs an embedded Keeper through `docker-compose/clickhouse/keeper.yaml`. Test 7 adds `keeper_map_path_prefix` to that configuration. The Compose job runs in the storage integration workflow with the other backends.

---

## 6. Upgrading an Existing Deployment

An existing deployment has the twelve objects created by the previous release. The first start of a binary with this mechanism behaves as follows:

- With `create_schema: true`, the plan finds ten missing OTLP columns in `spans`. They are all additive, so startup adds them without operator action. That is the point of putting the mechanism behind the existing flag. If `ttl` is set, the plan also reports a deferred `MODIFY TTL` on `spans` and `trace_id_timestamps`, because the `toDateTime(...)` wrapper of §8 changes the TTL expression that the templates render. Startup logs this difference, and `clickhouse-schema apply` resolves it.
- With `create_schema: false`, startup does nothing, exactly as today. Inserts fail until the operator applies the columns. The operator obtains the ten statements from `clickhouse-schema plan`, which the release notes name, instead of deriving them from a template diff.

The mechanism detects a mismatch between the configuration and the objects created a year ago. Today's startup does not expose that mismatch. A `ttl` changed since creation becomes a deferred `MODIFY TTL` without materialization. Startup logs it, and `clickhouse-schema apply` applies it. Configured retention then applies to parts as they merge. A partition that no longer merges retains its rows under the old TTL until an operator runs `MATERIALIZE TTL` on it (§2.3).

An engine that disagrees with the configuration produces a refusal at every start. For example, a local `MergeTree` table may exist under a configuration that now specifies `replicated`. The refusal remains until the operator rebuilds the table or corrects the configuration. A setting tuned by hand, including `index_granularity`, also produces a refusal at every start. A customized codec or TTL produces a deferred difference at every start until the template carries the customization or the operator accepts a standing warning.

These warnings and a multi-replica cluster without fan-out (§5.6) are the ways the upgrade can be noisier than before. The warnings report real differences, and the missing fan-out reports a real problem. Grants are the way the upgrade can fail outright. A `create_schema: true` deployment whose Jaeger user has only the `CREATE TABLE` grant needed by today's startup fails to start. Planning also needs `DROP TABLE`, `SHOW COLUMNS` and `SELECT ON system.data_skipping_indices` (§5.8). Additive `ALTER` statements need `ALTER`, which today's startup never issues. A multi-replica cluster needs `REMOTE`. A self-hosted `Replicated` database with a password on `default` needs a `collection_name` with cluster credentials (§5.6). The release notes list these requirements before the upgrade, and the startup error names the missing requirement.

---

## 7. Implementation Plan

1. **M1 — Diff engine and plan.** This milestone adds the `schemaReconciler.plan()` method, the scratch-table method, and the `TablePrefix` template parameter. It defines the vocabulary and refusals and adds the `clickhouse-schema plan` and `render` commands. It changes no live Jaeger object or startup behavior under either flag value. Its only writes create and drop scratch tables. Test 6 and the `plan` assertions of test 4 cover this milestone.
2. **M2 — Apply.** Startup applies additive statements with `create_schema: true` as specified in §5.3. It explicitly sets `alter_sync = 1` and verifies the changes through the polled second plan in §5.6. This milestone includes detection of `Replicated` and `Shared` databases, `throw_only_active`, and verification reads over the database's cluster. These are necessary for M3, the first real upgrade, to be safe on ClickHouse Cloud, where every database is a multi-replica `Shared` database. The `clickhouse-schema apply` command applies the whole plan without a lock, and this milestone adds `apply_deferred_changes`. Tests 1, 3 and 5 and the startup assertions of test 4 cover it. M1 and M2 supersede #9796. The guarantees delivered here are those in §5.5 for additive statements. M5 adds the lock guard rail for the operator command.
3. **M3 — OTLP span columns.** This milestone adds the ten columns to `create_spans_table.sql`, the `SpanRow` type and conversion wiring, and the corpus fixture. It includes a live upgrade test from a snapshot of the schema before this milestone, taken from git. Milestone 3 reduces #9799 to that scope. This is the mechanism's first real upgrade, tested against a live server before release.
4. **M4 — Release snapshots.** This milestone adds the `make` target in §5.7. It also generalizes the M3 snapshot into test 2.
5. **M5 — Clusters.** This milestone adds the `cluster` field, `ON CLUSTER` rendering, and `clusterAllReplicas` catalog reads over a named cluster. It also adds the `KeeperMap` lock for `clickhouse-schema apply` and test 7.
6. **M6 — RFC 0017 changes** arrive as template edits through this mechanism, each in its own PR. The `LowCardinality` change ships with its runbook.

---

## 8. Open Questions

- **Minimum ClickHouse version.** `MODIFY QUERY` no longer requires its experimental setting as of 23.11, and `keeper_map_strict_mode` became available in 23.4. The backend states no minimum version today. Three facts affect the choice. First, the current `event_attribute_metadata_mv` and `link_attribute_metadata_mv` fail to create on 24.3 through 24.12 under the default analyzer. They return `UNKNOWN_IDENTIFIER` for the nested attribute keys. They create on 25.3, so a fresh install needs 25.3 regardless of this RFC's decision. Second, a `Replicated` database requires an experimental setting on 24.3. Third, today's rendered TTL clauses are rejected with `BAD_TTL_EXPRESSION` on 24.3 through 25.3 and accepted on 25.12. These clauses are `start_time + INTERVAL n SECOND` over a `DateTime64(9)` column on `spans` and `end + INTERVAL n SECOND` over a `SimpleAggregateFunction(max, DateTime64(9))` column on `trace_id_timestamps`. This RFC proposes a minimum version of 25.3, documented with M1. A separate change before M1 wraps the column in `toDateTime(...)` in both templates, because for existing databases the wrapper is a deferred TTL difference rather than a rewrite, and it keeps 25.3 rather than 25.12 as the floor.
- **`apply_deferred_changes` as a setting.** This setting lets a single-node installation retain today's hands-off behavior for every change. The alternative omits the setting and requires even single-node operators to run `clickhouse-schema apply` for deferred changes. That design is simpler but adds one operator step.
- **A validate-only startup mode.** A migration runner inside a server binary conventionally has three modes. It can apply changes at startup, validate at startup and fail or warn if the schema is behind, or do nothing. This design provides the first and third through `create_schema`. The validating form is `clickhouse-schema plan` run as an init container or a CI step, because a plan computed inside the collector under `create_schema: false` would need the scratch-table grants that the operator-managed configuration exists to withhold. This RFC proposes deferring a third `create_schema` value, which would plan at startup and only log, until an operator asks for it, since adding it later costs nothing and §5.3 is simpler with two rows than with three.

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
11. [Atlas: declarative vs. versioned workflows](https://atlasgo.io/concepts/declarative-vs-versioned): the generated-migration workflow that §4.4 describes.
12. [Ent schema migration](https://entgo.io/docs/migrate) and [GORM AutoMigrate](https://gorm.io/docs/migration.html): additive reconcilers that run inside a Go application at startup and never drop by default.
13. [ClickHouse temporary tables](https://clickhouse.com/docs/sql-reference/statements/create/table#temporary-tables): the session-scoped tables that §2.4 rules out for scratch use.
14. [ClickHouse `alter_sync`](https://clickhouse.com/docs/operations/settings/settings#alter_sync): the wait modes of a replicated `ALTER`, and why §5.6 sets it to `1` explicitly.

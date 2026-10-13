# RFC 0019: ClickHouse Schema Migrations

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-10-13
- **Last Updated:** 2026-10-13
- **Issue:** [#9789](https://github.com/jaegertracing/jaeger/issues/9789)
- **Related:** [ADR-008 (ClickHouse storage schema)](../adr/008-clickhouse-storage-schema.md) · [RFC 0017 (schema tuning adopted from ClickStack)](./0017-clickhouse-schema-vs-clickstack.md) · [#9796 (migration framework PR)](https://github.com/jaegertracing/jaeger/pull/9796) · [#9799 (OTLP span columns PR)](https://github.com/jaegertracing/jaeger/pull/9799) · [#9813 (configurable table engine)](https://github.com/jaegertracing/jaeger/pull/9813)

---

## Abstract

The ClickHouse backend creates its twelve tables and materialized views at startup from one `CREATE ... IF NOT EXISTS` template per object, and has no way to change an object that already exists. The first change to the schema since it reached users, the missing OTLP span columns of #9789, therefore has to establish how the schema is amended from now on. This RFC describes the mechanism as it stands, states what a migration mechanism has to satisfy for this backend, and proposes to adopt [golang-migrate](https://github.com/golang-migrate/migrate) with a layout in which each schema version is a directory holding only the statements for the objects that version changes, rendered through the existing templates before golang-migrate sees them. The library is used for what it is good at, version bookkeeping and ordered application, and its one-file-per-version convention is satisfied by an in-memory file system assembled at startup rather than by committing a full copy of the schema per version, which is the organization the Cassandra backend was built on and the one this RFC sets out to avoid.

---

## 1. Motivation

Three things force the question now.

1. **A schema change is pending.** #9789 adds ten columns to the `spans` table. Against an existing deployment the startup statements are no-ops, so without a migration mechanism the columns never appear and the writer fails on its first insert after the upgrade.
2. **RFC 0017 queues more.** The codecs, `LowCardinality` wrappers, table settings and skip indexes that RFC 0017 adopts are each an `ALTER TABLE`, and the attribute layout decision it defers may end in a new column type. The backend is going to change its schema several times in the next few releases, and each change needs the same path.
3. **Two PRs already implement one answer.** #9796 and #9799 adopt golang-migrate along the lines sketched in #9789 and put the whole baseline schema in a single `000001_initial_schema.up.sql`. Review of those PRs showed that the file layout, the loss of the template parameters, and the test design need deciding before the code, which is what an RFC is for.

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
| Add a skip index | `ALTER TABLE ... ADD INDEX IF NOT EXISTS` | metadata only; existing parts indexed on merge or `MATERIALIZE INDEX` | propagates |
| Change a codec or `LowCardinality` | `ALTER TABLE ... MODIFY COLUMN` | rewrites the column on merge | propagates |
| Change a table setting | `ALTER TABLE ... MODIFY SETTING` | metadata only | propagates |
| Change TTL | `ALTER TABLE ... MODIFY TTL` | schedules a `materialize_ttl` mutation over every part² | propagates |
| Change a view's query | `ALTER TABLE <view> MODIFY QUERY` | metadata only | propagates |
| Change `ENGINE`, `ORDER BY`, `PARTITION BY` | not possible | requires a new table and a copy | n/a |

- ¹ The column is added to each part's metadata; existing parts read the column's default until they are merged.
- ² The mutation rewrites parts whose rows the new expression expires, which for a large `spans` table is hours of background I/O. It is the reason TTL belongs in `CREATE` and not in a per-startup `ALTER`.

Two properties of replicated deployments matter. An `ALTER` on a `Replicated*` table is recorded in Keeper and applied on every replica, so a migration issued through one connection reaches the whole shard. A `CREATE TABLE` is not: it creates the table only on the server the client reached, and reaching the others needs `ON CLUSTER` or a connection to each. A mechanism whose later versions are `ALTER`s therefore has one cluster-sensitive step, the first, and today's `CREATE`-at-startup has that same step and nothing else.

### 2.4 How the other backends do it

Cassandra was described above. Elasticsearch and OpenSearch have no comparable problem: their mapping is an index template that the binary installs at startup, and because indices roll daily, a new mapping applies to the next index without altering the previous one. Badger and memory have no persistent schema. ClickHouse is the first Jaeger backend whose tables are long-lived and alterable, which is why neither precedent transfers.

---

## 3. Requirements

The mechanism is judged against these criteria, which also form the rows of the comparison in §6.

1. **Per-object deltas.** A schema version holds statements for the objects it changes and nothing else. A change to one column of one table is one short file.
2. **One source of truth.** What a fresh install creates and what an upgraded install ends up with are the same DDL, derived from one set of files, not two paths kept equal by discipline.
3. **Template parameters survive.** `ttl` and `table_engine` are properties of the deployment, not of the schema version, and every `CREATE` has to render them. A replicated deployment that gets local tables because the migration file hard-codes `MergeTree` is a silent data-loss configuration.
4. **Recorded version.** The database says which version it is at, so the binary can refuse to run against a schema it does not understand, and an operator can tell what a cluster has.
5. **Safe with concurrent instances.** Several Jaeger instances start together against one database. The result has to be correct whoever finishes second.
6. **Operator-managed schema stays possible.** With `create_schema: false` the operator applies the DDL by hand, and the same files have to serve them.
7. **Readable current schema.** A developer changing the writer can find out what the `spans` table looks like today without replaying history in their head.
8. **Small dependency footprint.** The backend pulls in no database drivers it does not use.

---

## 4. golang-migrate

#9789 names golang-migrate and the two open PRs use it, so its contract is worth stating precisely, because the layout question in §5 turns on what the library requires rather than on what its examples show.

### 4.1 The contract

The library's core is `migrate.Migrate`, which walks a `source.Driver` and applies each version through a `database.Driver`. The source interface is seven methods:

```go
type Driver interface {
    Open(url string) (Driver, error)
    Close() error
    First() (version uint, err error)
    Prev(version uint) (prevVersion uint, err error)
    Next(version uint) (nextVersion uint, err error)
    ReadUp(version uint) (r io.ReadCloser, identifier string, err error)
    ReadDown(version uint) (r io.ReadCloser, identifier string, err error)
}
```

A version is an unsigned integer. For each version the source yields one reader of SQL for the up direction and, optionally, one for down. That is the whole requirement: nothing says what a version's SQL contains, how many statements it holds, or where the bytes come from.

The `{version}_{name}.up.sql` filename convention belongs to the `source/iofs` adapter, which lists one directory of an `fs.FS`, parses each name with the regular expression `^([0-9]+)_(.*)\.(down|up)\.(.*)$`, ignores names that do not match, and rejects two files with the same version and direction. Any `fs.FS` works, including one built in memory.

The `database/clickhouse` driver takes a `*sql.DB` and a `Config`:

```go
type Config struct {
    DatabaseName          string
    ClusterName           string // adds ON CLUSTER to the version table's own DDL only
    MigrationsTable       string // default "schema_migrations"
    MigrationsTableEngine string // default "TinyLog"
    MultiStatementEnabled bool   // split the version's SQL on ';' and run each statement
    MultiStatementMaxSize int    // default 10 MB
}
```

`WithInstance` creates the version table as a side effect if it is missing. The table has columns `version Int64, dirty UInt8, sequence UInt64`; each run inserts a row with `dirty = 1` before applying the version and another with `dirty = 0` after, and `Version()` reads the newest row by `sequence`. `Lock()` is a process-local atomic flag, so two processes are never serialized against each other. `Force(version)` writes a version row without running anything. `Up()` returns `ErrNoChange` when nothing is pending and `ErrDirty` when the newest row is dirty.

### 4.2 Why a library at all

The version table, the ordered walk and the dirty flag are about a hundred lines to write, and the comparison below is why the library is still the better choice.

| Criterion | Hand-rolled version table | golang-migrate |
| --- | --- | --- |
| Version table, ordered walk, dirty flag | 🟨¹ | 🟢 |
| Takes SQL from an `fs.FS` | 🟢 | 🟢 |
| Dependency footprint | 🟢 | 🟢² |
| Mutual exclusion across processes on ClickHouse | ❌³ | ❌³ |
| Version table engine configurable | 🟢 | 🟢 |
| Familiar to operators | ❌ | 🟢 |

- ¹ About a hundred lines to write, and then to maintain, test and explain.
- ² Drivers are opt-in by import path; importing `database/clickhouse` and `source/iofs` compiles seven packages and no third-party module beyond `golang-migrate/migrate/v4` itself.
- ³ ClickHouse has no advisory locks, so no library can offer them; §5.4 makes every step idempotent instead.

golang-migrate wins on the two rows where the options differ. The hand-rolled option loses on familiarity, and that row matters because operators who manage the schema themselves (§5.6) are better served by a table and a convention they have seen elsewhere. pressly/goose would score the same as golang-migrate on every row here and was not evaluated further, because #9789 and the two open PRs already use golang-migrate. Atlas was set aside because it is declarative and CLI-driven, which puts the migration planner outside the binary.

---

## 5. Design

### 5.1 Layout

Each schema version is a directory. A directory holds one file per object that the version creates or alters, numbered so that lexical order is dependency order.

```
internal/storage/v2/clickhouse/sql/schema/
  v001/
    01_spans_table.sql                 (today's create_spans_table.sql, frozen)
    02_services_table.sql
    03_services_mv.sql
    04_operations_table.sql
    05_operations_mv.sql
    06_trace_id_timestamps_table.sql
    07_trace_id_timestamps_mv.sql
    08_attribute_metadata_table.sql
    09_attribute_metadata_mv.sql
    10_event_attribute_metadata_mv.sql
    11_link_attribute_metadata_mv.sql
    12_dependencies_table.sql
  v002/
    01_spans_otlp_fields.sql           (ALTER TABLE spans ADD COLUMN IF NOT EXISTS ..., ten columns)
  v003/
    01_spans_codecs.sql                (RFC 0017: MODIFY COLUMN ... CODEC(...))
    02_spans_settings.sql              (RFC 0017: MODIFY SETTING ttl_only_drop_parts = 1)
testdata/
  schema.golden.sql                    (SHOW CREATE TABLE of every object after every version)
```

Version 1 is the twelve files that exist today, moved without edits. Every later version is a delta. The directory number is the golang-migrate version and is zero-padded to three digits so that lexical and numeric order agree, matching the Cassandra `v001` convention. Files inside a directory are prefixed with two digits for the same reason, and the prefix replaces the ordered list that `newSchemaBuilder` holds in Go today, so a version directory is self-describing.

Every file is a template. A file without template directives renders as itself, so the table/view distinction that `newSchemaBuilder` tracks disappears.

### 5.2 Rendering and assembly

At startup, the backend renders every directory into one golang-migrate version and hands the result to `iofs`:

```go
//go:embed schema
var schemaFS embed.FS

// migrationSource renders each schema/vNNN directory with the deployment's parameters
// and presents it to golang-migrate as version NNN.
func migrationSource(params schemaTemplateParams) (fs.FS, error) {
    out := fstest.MapFS{}
    versions, err := fs.ReadDir(schemaFS, "schema")
    if err != nil {
        return nil, err
    }
    for _, dir := range versions {
        n, err := strconv.Atoi(strings.TrimPrefix(dir.Name(), "v"))
        if err != nil {
            return nil, fmt.Errorf("schema directory %q is not vNNN: %w", dir.Name(), err)
        }
        files, err := fs.ReadDir(schemaFS, path.Join("schema", dir.Name()))
        if err != nil {
            return nil, err
        }
        var body strings.Builder
        for _, f := range files {
            rendered, err := renderTemplate(schemaFS, path.Join("schema", dir.Name(), f.Name()), params)
            if err != nil {
                return nil, err
            }
            body.WriteString(rendered)
            body.WriteString(";\n")
        }
        out[fmt.Sprintf("%06d_%s.up.sql", n, dir.Name())] = &fstest.MapFile{Data: []byte(body.String())}
    }
    return out, nil
}
```

`fs.ReadDir` returns entries sorted by name, which is the order the prefixes define. The ClickHouse driver's `MultiStatementEnabled` splits the concatenated body on `;` and runs each statement, so a version with twelve objects and a version with one are handled alike. `renderTemplate` is the `loadTemplate` function that exists today, reading from the embedded FS instead of a string constant.

This is the part of the design that makes requirements 1 through 3 compatible with the library. The committed files stay per object and templated; the assembled file is what golang-migrate's naming convention asks for; and the deployment's engine and TTL are rendered into every `CREATE` exactly as they are now.

### 5.3 Startup behavior

| `create_schema` | Version table | Behavior |
| --- | --- | --- |
| `true` | any | `Up()` applies pending versions and records each. `ErrNoChange` is success. |
| `true` | newest row dirty | Another instance is mid-version, or a previous run crashed. Retry `Up()` with backoff for a bounded time (15 s). If the row is still dirty, treat the version as abandoned: `Force` the version table back one version and run `Up()` once more, which is safe because every statement is idempotent (§5.4). If that fails, fail startup with the version and the statement that failed. |
| `false` | present, version equals the binary's | Start. |
| `false` | present, version below the binary's | Fail: the operator has not applied the pending versions; the message names them and says how to print them (§5.6). |
| `false` | present, version above the binary's | Fail: the schema is newer than the binary understands. |
| `false` | absent | Warn and start. A deployment whose tables predate the version table has nothing to compare against, and refusing to start would break every existing operator-managed deployment on upgrade. The warning says how to record the version. |

The `create_schema: false` path reads the version table with a plain query rather than through `WithInstance`, because `WithInstance` creates the table if it is missing, and an operator who owns the schema has not asked for that write.

### 5.4 Rules for writing a version

These are the conventions that make the mechanism safe without a cross-process lock, and they are enforced by the tests in §5.7 where a test can enforce them.

1. **Every statement is idempotent.** `CREATE ... IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `ADD INDEX IF NOT EXISTS`, `MODIFY ...` where re-applying the same value is a no-op. Two instances applying the same version both succeed, and the version table gets two rows, which is harmless because `Version()` reads the newest.
2. **Recovery from a dirty version is a rerun.** Because every statement is idempotent, an instance that crashed mid-version leaves work that the next start simply finishes, which is what the dirty-row handling in §5.3 relies on. Two instances that both time out and both rerun the version still end in the correct schema.
3. **No down files.** `DROP COLUMN` destroys data that the binary has no business destroying on its own. Rolling back is deploying the previous binary, and the next rule is what makes that work.
4. **Expand before contract.** A version adds columns, indexes and settings; it never removes or renames anything that the previous release reads or writes. Removing a column is a version in a release after the one that stopped using it. A rolling deploy therefore always has an older binary running against a schema one version ahead, and the older binary has to tolerate that, which `INSERT` with an explicit column list and `SELECT` with explicit columns already do.
5. **Metadata-only or merge-time.** A version may add columns and indexes, modify codecs, settings and view queries. It may not `MODIFY TTL`, change `ORDER BY`, `PARTITION BY` or `ENGINE`, or populate a column with `UPDATE`. Each of those rewrites data, is hours long on a real table, and is an operator procedure with a runbook, not a startup step. A view whose query changes uses `MODIFY QUERY`, never drop-and-recreate, because inserts during the gap would bypass the view.
6. **A released version is frozen.** Once a release ships `v00N`, its files are never edited. A change to what `v00N` created is `v00N+1`. The golden-file test in §5.7 catches an edit to a released version because the golden file would change without a new directory.

### 5.5 Replicated clusters

The version table is created by the driver with `MigrationsTableEngine`, which defaults to `TinyLog`, a replica-local engine. On a cluster each replica would then hold its own idea of the version, and which one a Jaeger instance sees would depend on which replica the load balancer chose. The backend sets `MigrationsTableEngine` from the same `engineClause(mergeTreeFamily)` that renders the data tables, so the version table is `ReplicatedMergeTree` exactly when the data tables are.

The `ON CLUSTER` question is left open (§9). `ClusterName` in the driver's config only decorates the version table's own `CREATE`; the schema files would need their own `{{ .OnCluster }}` clause, and ClickHouse Cloud and single-shard replicated deployments, which are the common cases, do not need it because `ALTER` propagates and the initial `CREATE` runs on each replica as each Jaeger instance connects. Today's behavior is no worse, and a `cluster_name` option can be added as its own change.

### 5.6 Operator-managed schema

With `create_schema: false` the operator applies the same DDL by hand. The binary makes that possible with a command that prints what it would run:

```
jaeger ... --config config.yaml print-clickhouse-schema [--from-version N]
```

The output is the rendered versions in order, each followed by the `INSERT INTO schema_migrations` row that records it, so an operator pastes the output into `clickhouse-client` and ends up in the same state `Up()` would have produced, version row included. The command renders with the configuration file's `ttl` and `table_engine`, which is what makes the printed DDL match the deployment rather than a default. An existing deployment that predates the version table records its version by applying the printed `INSERT` for version 1 alone, which is what the warning in §5.3 points at. The Cassandra backend ships the equivalent as a separate `jaeger-cassandra-schema` binary; here it is a subcommand of the one binary, because the rendering depends on the same configuration.

### 5.7 Tests

Three tests, all against the live ClickHouse that the integration suite already starts.

1. **Golden schema.** Apply every version to an empty database, dump `SHOW CREATE TABLE` for each object, and compare with `testdata/schema.golden.sql`, updating it under `-update`. The golden file is how a developer reads the current schema (requirement 7), and a change to it without a new version directory is the signal that a released version was edited.
2. **Fresh equals migrated.** For each version N above 1: apply versions 1 through N−1 and then N, dump; drop everything; apply 1 through N in one `Up()`, dump; require equality. With one version the two runs are identical by construction, which is the harness proving itself before version 2 lands. Run once with `merge_tree` and `ttl: 0` and once with `ttl` set, so the TTL clause is covered by the live test; the replicated rendering is covered by the unit test on `migrationSource`, because the CI ClickHouse runs without Keeper.
3. **Idempotence.** Apply every version twice and require the second `Up()` to return `ErrNoChange` after `Force(0)`, which proves rule 1 of §5.4 for every statement shipped.

The unit tests cover `migrationSource`: directory naming, file ordering, rendered engine and TTL for both engine families, and a directory whose name does not parse.

---

## 6. Alternatives

| Criterion (§3) | A. One baseline file per version (#9789, #9796) | B. One golang-migrate version per table | C. Living templates for fresh installs, deltas for upgrades¹ | D. Full schema copy per version (Cassandra) | **E. Rendered delta directories (§5)** |
| --- | --- | --- | --- | --- | --- |
| 1. Per-object deltas | 🟨² | 🟢 | 🟢 | ❌ | 🟢 |
| 2. One source of truth | 🟢 | 🟢 | 🟨³ | 🟢 | 🟢 |
| 3. Template parameters survive | ❌⁴ | 🟢⁵ | 🟢 | 🟢 | 🟢 |
| 4. Recorded version | 🟢 | 🟨⁶ | 🟢 | ❌ | 🟢 |
| 5. Safe with concurrent instances | 🟢 | 🟢 | 🟢 | n/a | 🟢 |
| 6. Operator-managed schema | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 |
| 7. Readable current schema | 🟨⁷ | 🟨⁷ | 🟢 | 🟢 | 🟨⁷ |
| 8. Dependency footprint | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 |

- ¹ A fresh database renders today's templates and calls `Force(N)`; an existing database applies the deltas.
- ² Version 2 onward would be deltas, but version 1 is one long file that nobody reads per object, and the per-object organization of the templates is lost at the moment the framework is adopted.
- ³ The templates and the deltas have to agree, and only the fresh-equals-migrated test enforces it; a template edit with no delta passes review and breaks upgrades.
- ⁴ As committed, the files are plain SQL with `MergeTree` hard-coded and no TTL, applied as bytes. TTL is bolted on with `MODIFY TTL` on every start, which §2.3 shows to be a mutation, and the engine cannot be bolted on at all.
- ⁵ Only if the files are rendered, as in E; B as such says nothing about rendering.
- ⁶ A fresh install lands at version 12 before any change has happened, and the version number counts tables rather than schema revisions.
- ⁷ Through the golden dump, which is generated rather than authored but is plain ClickHouse DDL.

E is the recommendation. It is the only column that is green on the first four rows together, and those are the rows that decided it: per-object deltas are what the Cassandra experience argues for, one source of truth is what makes the fresh-equals-migrated test a check rather than a crutch, and template parameters are a correctness requirement since #9813. The row E loses, readable current schema, is the ordinary trade of every migration framework and is covered by the golden file. C is the serious contender and loses on row 2 alone, but that row is the one that fails silently. A is what the issue proposed and what the open PRs implement, and this RFC replaces that part of the issue.

---

## 7. Upgrading an Existing Deployment

An existing deployment has the twelve objects and no version table. On the first start of a binary with this mechanism:

- With `create_schema: true`, `WithInstance` creates the version table, `Up()` finds no rows and runs version 1, every statement of which is `CREATE ... IF NOT EXISTS` against a table that exists, so nothing changes and the row for version 1 is written. Version 2 then runs its `ADD COLUMN IF NOT EXISTS` statements. The deployment is migrated with no operator action, which is the point of putting the mechanism behind the existing flag.
- With `create_schema: false`, the binary warns that the version table is absent and starts (§5.3). The operator applies the printed version 1 row to silence the warning and the printed version 2 DDL to get the columns, in either order.

Version 1 renders the deployment's current `ttl` and `table_engine`, but because its statements are no-ops against existing tables, a mismatch between the configuration and what was created a year ago is not detected here, exactly as it is not today. Detecting it is a separate feature if it is wanted.

---

## 8. Implementation Plan

1. **M1 — Framework.** Move the twelve files to `schema/v001/` with ordinal prefixes, add `migrationSource`, replace `schemaBuilder.build` with `migrate.Up()` behind `create_schema`, set `MigrationsTableEngine` from the engine clause, implement the §5.3 table, and add the three tests of §5.7 with one version. No user-visible change for a deployment that already has the tables. #9796 is the basis for this milestone and is restructured to it.
2. **M2 — OTLP span columns.** `schema/v002/01_spans_otlp_fields.sql`, the `SpanRow` and conversion wiring, the corpus fixture, and the first fresh-equals-migrated pair. #9799 rebased onto M1.
3. **M3 — Operator path.** The `print-clickhouse-schema` command of §5.6 and the documentation page for operator-managed schemas.
4. **M4 — RFC 0017 changes** land as `v003` and later through this mechanism, each as its own PR.

---

## 9. Open Questions

- **`ON CLUSTER`.** Whether to add a `cluster_name` setting that decorates every DDL statement, for sharded self-hosted clusters whose replicas are not each reached by a Jaeger instance. Deferred until a user needs it; §5.5 explains why the common deployments do not.
- **One-shot migration mode.** Large fleets may prefer an init container that applies migrations once before the pods roll, in the manner of Flyway on Kubernetes. The `print-clickhouse-schema` command plus `clickhouse-client` covers it; a dedicated `migrate-only` exit is a small addition if the demand appears.
- **Squashing.** A fresh install replays every version. At a dozen `ALTER`s on empty tables this is milliseconds and not worth a baseline rewrite; if the history grows long, a squash to a new baseline is safe only once every supported deployment is past the squash point, and that policy is written when it is needed.

---

## References

1. [golang-migrate/migrate](https://github.com/golang-migrate/migrate: the migration library, its `source.Driver` interface and the `database/clickhouse` driver.
2. [ClickHouse ALTER TABLE reference](https://clickhouse.com/docs/sql-reference/statements/alter: which changes are metadata-only and which rewrite data.
3. [ClickHouse `ALTER TABLE ... MODIFY TTL` and `materialize_ttl_after_modify`](https://clickhouse.com/docs/sql-reference/statements/alter/ttl: why TTL changes are mutations.
4. [Jaeger Cassandra schema management](../../internal/storage/v1/cassandra/schema/README.md: the per-version full-copy layout this RFC avoids.
5. [Issue #9789](https://github.com/jaegertracing/jaeger/issues/9789: the OTLP column gap and the original migration sketch.

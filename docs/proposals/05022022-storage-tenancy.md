# Storage tenancy

* **Owners:**:
  * `@esnible`

## TL;DR

This proposal supports a new optional `x-tenant` header to Jaeger's HTTP and GRPC interfaces.

This proposal adds a `"tenant"` key to the Jaeger `Context` that accompanies spans through their processing pipeline down to the `spanstore.Writer`.
The `"tenant"` key also accompanies queries to their `spanstore.Reader`.

## Why

To allow a single Jaeger deployment to store and query traces for multiple entities.

## Goals

* Allow Jaeger to service multiple tenants/customers
* Let SpanWriter implementations make their own decision to offer tenancy, and implementation strategies.
* Let deployers enumerate accepted tenants

## Non-Goals

* A security mechanism for tenants
* Conventions/APIs/libraries to help storage writers implement tenancy.

## How

The how is split into the following sections:
* Jaeger implementation - describes how users access data stored in Jaeger.
* Jaeger configuration - describes how Jaeger components are deployed.

### Jaeger implementation

Storage implementations that wish to support tenancy have may use whatever strategy they choose.

For example, 

- create one NoSQL connection per-tenant, to isolated database hosts
- pass the tenant to a NoSQL service through a header
- Incorporate the tenant into a NoSQL index name
- Incoporate the tenant into a NoSQL value

Jaeger does not prescribe an implementation.

The memory storage example uses a `Map[tenant]Tenant`, with each `Tenant` being an independent memory span store.

If a `spanstore.Writer` also implements the new `spanstore.TenantValidator`
interface, and spans will be passed through the validator before they are
queued and processed.

### Jaeger configuration

The tenant configuration is part of the storage configuration.

For example, the memory storage offers `--memory.max-tenants=12` and `--memory.valid-tenants=acme,wonka,stark-industries`.

For memory storage, `--memory.valid-tenants` has precedence and will reject any batches or queries that do not include a header with a value in the list.

`--memory.max-tenants` defaults to 10, and can be set to 0 to allow all tenants.  When non-zero, it rejects any new tenants seen after the first N.  It is intended as a demonstration, but also so that a Jaeger deploy can be configured to reject an unbounded number of traces (when combined with `--memory.max-traces`.)

# cmd/jaeger/components/

This is the **bridge layer** between the public facade (`components/`) and the
internal implementation (`cmd/jaeger/internal/`).

Go's `internal` package restriction means only code within the `cmd/jaeger/` subtree
can import `cmd/jaeger/internal/`. The public `components/` directory is outside that
subtree, so it cannot import the internal packages directly. This bridge sits inside
the subtree and re-exports `NewFactory` variables that the public facades can reference.

## Pattern

Each package here contains a single `factory.go`:

```go
package jaegerstorage

import impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"

var NewFactory = impl.NewFactory
```

The public facade in `components/extension/jaegerstorage/` then aliases this bridge:

```go
package jaegerstorage

import impl "github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerstorage"

var NewFactory = impl.NewFactory
```

This double-dispatch is only needed for Jaeger-specific components. Third-party
components bypass this layer entirely via `components/ext/`.

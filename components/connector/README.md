# components/connector/

Facade packages for OTel Collector connector components included in Jaeger.

Each sub-package exports `NewFactory` which returns the connector's factory.
Package names match the upstream module names exactly (e.g., `forwardconnector`,
`spanmetricsconnector`) so that import aliases are unnecessary.

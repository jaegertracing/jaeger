# Configuration Documentation Generation

This document explains how to generate configuration reference markdown files from Jaeger's source code.

## Prerequisites

- Go 1.20+
- `jsonschema2md` tool (install via npm)
- Jaeger source code

```bash
npm install -g @adobe/jsonschema-tools
```

## Generation Process

**Generating Markdown file**
```bash
$ jsonschema2md cmd/jaeger/internal/configdocs/cmd/jaeger-config-schema.json cmd/jaeger/docs/migration/configdocs.md
# generated output is written to docs/migration
```

## How It Works

The tool:
1. Analyzes Jaeger's configuration structs via AST
2. Generates JSON Schema with package-qualified type names
3. Uses `jsonschema2md` to create human-readable docs

## Troubleshooting

- **Permission Denied**: Use `sudo` for npm installs
- **Missing Dependencies**: `go mod tidy`
- **Broken Links**: Ensure fully qualified type names
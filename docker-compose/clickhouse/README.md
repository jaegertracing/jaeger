# ClickHouse Composes

This directory contains versioned Docker Compose files for ClickHouse LTS releases.

## Directory Layout

```
clickhouse/
├── README.md
├── lts-previous/       # Previous LTS line (25.8.x — supported until early 2026)
│   └── docker-compose.yml
└── lts-latest/         # Current LTS line (26.3.x — supported until early 2027)
    └── docker-compose.yml
```

Versioned subdirectories mirror the pattern used for OpenSearch in `docker-compose/opensearch/`.

## LTS Policy

- ClickHouse LTS releases receive bug and security backports for approximately 12 months.
- Jaeger tests two consecutive LTS lines concurrently:
  - `lts-previous`: the previous LTS line (still receiving backports).
  - `lts-latest`: the latest LTS line (current stable).
- Monthly stable releases (e.g. `25.12`) publish every four weeks but receive **no backports**. They are not used because CI would break unpredictably between releases.

## Rotation Process

1. When a new LTS line is released (typically every 4–6 months):
   - Rename `lts-latest` → `lts-previous` (update its Renovate rule and `allowedVersions`).
   - Create a new `lts-latest` directory with the new LTS version.
   - Add a Renovate rule for the new LTS line.
2. When the oldest LTS line reaches end of life:
   - Remove the `lts-previous` directory and its Renovate rule.

## Upgrade Process

- **Patch/digest updates** are handled automatically by Renovate — each LTS line has its own `packageRules` entry in `renovate.json` with `"matchUpdateTypes": ["patch", "digest"]`.
- **Major/minor LTS upgrades** (e.g. 25.8 → 26.3 or 26.3 → 27.1) require the manual rotation process above.

## Testing

Both LTS lines are tested in CI. To run tests locally:

```bash
# Test previous LTS line with e2e mode
bash scripts/e2e/clickhouse.sh e2e lts-previous

# Test latest LTS line with direct mode
bash scripts/e2e/clickhouse.sh direct lts-latest
```

## Renovate Interaction

Each LTS line has an `enabled: true` `packageRules` entry that overrides the blanket
`"docker-compose/**/docker-compose.y*ml"` disable rule. Rules are restricted to
`patch` and `digest` update types so Renovate will never propose a cross-LTS-line
upgrade.

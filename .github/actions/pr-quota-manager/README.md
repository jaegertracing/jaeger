# PR Quota Manager

Limits how many pull requests a new contributor can have open at once, by labelling the ones over quota with `pr-quota-reached` and explaining why in a comment. `jaegertracing/jaeger` and `jaegertracing/jaeger-ui` both apply the policy through the composite action in this directory.

The logic lives in [`pr-quota-manager.js`](pr-quota-manager.js) in this directory, next to its tests.

## Using the action

```yaml
permissions:
  pull-requests: write
  issues: write

steps:
  - uses: actions/checkout@v7 # only when calling the action from this repository
  - uses: jaegertracing/jaeger/.github/actions/pr-quota-manager@<commit-sha>
    with:
      token: ${{ github.token }}
```

| Input | Required | Default | Purpose |
|---|---|---|---|
| `token` | yes | — | Token used to label and comment. Needs `issues: write` and `pull-requests: write`. |
| `username` | no | PR author | Whose quota to process. Only manual runs need to set it. |
| `dry-run` | no | `false` | Log the intended labels and comments without writing anything. |

Labels and comments on a pull request go through the `/issues/{n}/labels` and `/issues/{n}/comments` endpoints, so the token needs `issues: write` — `pull-requests: write` alone is not enough, and a token missing it fails every write with `403 Resource not accessible`.

Consumers outside this repository pin a commit SHA, so a change to the script reaches them only when that pin is bumped, which Renovate raises as an ordinary dependency pull request.

## Listing Open PRs by Author

Use the utility script in `.github/scripts` to see all open PRs grouped by author. It needs a `GITHUB_TOKEN` in the environment:

```bash
cd .github/scripts
npm ci
node list-open-prs-by-author.js [owner] [repo]
```

This is useful for:
- Identifying which users need quota processing
- Planning backfills of the quota system
- Seeing which PRs are already quota-blocked

**CSV output for scripting:**
```bash
FORMAT=csv node list-open-prs-by-author.js > prs.csv
```

## Output

The log of a run displays:

1. **History Audit**: Summary of merged PR count (up to 3 merged PRs for quota calculation)
2. **Current Stats**: Merged count, calculated quota, and open PR count
3. **Processing Actions**: Each PR being blocked/unblocked
4. **Summary**: Total counts of blocked, unblocked, and unchanged PRs

### Example Output

```
=== Processing Quota for: @newuser ===

📜 History Audit:
  No merged PRs found.

📊 Current Stats:
  User has 0 merged PRs. Current Quota: 1. Currently Open: 3.

🔄 Processing Open PRs:

  ℹ️  PR #123 unchanged (active)
  ✅ Labeled PR #124 as blocked (Position: 2/3, Quota: 1)
  ✅ Labeled PR #125 as blocked (Position: 3/3, Quota: 1)

✅ Processing Complete for @newuser

=== Author quota summary ===
Blocked: 2 PRs
Unblocked: 0 PRs
Unchanged: 1 PRs
```

## Running Tests

To run the unit tests:

```bash
cd .github/scripts
npm test
```

To run tests with coverage:

```bash
npm test -- --coverage
```

## Quota Rules

The script applies the following quota rules:

| Merged PRs | Quota |
|-----------|-------|
| 0 | 1 |
| 1 | 2 |
| 2 | 3 |
| 3+ | 10 |

## How It Works

1. **Fetches PRs** by the target author (all open PRs + up to 3 merged PRs for quota calculation)
2. **Calculates quota** based on the number of merged PRs
3. **Identifies open PRs** and sorts them by creation date (oldest first)
4. **Applies labels** to PRs based on quota:
   - PRs within quota: Remove `pr-quota-reached` label (if present)
   - PRs exceeding quota: Add `pr-quota-reached` label
5. **Posts comments** (only on state changes to avoid spam):
   - Blocking comment when PR first gets blocked
   - Unblocking comment when PR is moved to active queue

## Integration with GitHub Actions

`.github/workflows/pr-author-quota.yml` calls the action for the existing
author quota on pull request opened, closed, reopened, or pushed events, and on
manual workflow dispatch. Manual runs pass `username`, and optionally `dryRun`.

The action runs the script through `actions/github-script` with the repository's built-in `GITHUB_TOKEN`. The `pull_request_target` trigger runs in the base repository context, so that token carries the workflow's declared write permissions even for pull requests from forks.

`jaegertracing/jaeger-ui` calls the same action at a pinned SHA rather than keeping its own copy of the script.

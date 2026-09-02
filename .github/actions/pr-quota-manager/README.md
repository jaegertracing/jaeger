# PR Quota Manager

Limits how many pull requests a new contributor can have open at once, by labelling the ones over quota with `pr-quota-reached` and explaining why in a comment. `jaegertracing/jaeger` and `jaegertracing/jaeger-ui` both apply the policy through the composite action in this directory.

The logic lives in [`.github/scripts/pr-quota-manager.js`](../../scripts/pr-quota-manager.js), next to the dependencies and jest setup its command line and tests need. This directory holds the action wrapper and this guide.

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

Everything below covers running the script directly from the command line, for testing and troubleshooting.

## Prerequisites

1. **Node.js** (version 16 or higher)
   ```bash
   node --version
   ```

2. **GitHub Personal Access Token** with the following permissions:
   - `repo` (Full control of private repositories)
   - `public_repo` (Access public repositories) - if working with public repos only

   Create a token at: https://github.com/settings/tokens.
   Store the value in a file, e.g. `~/.github_token`.
   Then set the environment variable:
   ```bash
      read -r GITHUB_TOKEN < ~/.github_token
      export GITHUB_TOKEN
   ```

3. **Install Dependencies**

   Navigate to the `.github/scripts` directory and install dependencies:
   ```bash
   cd .github/scripts
   npm ci
   ```

## Running the Script

### Basic Usage

```bash
node pr-quota-manager.js <username> [owner] [repo]
```

### Parameters

- `username` (required): The GitHub username to process quota for
- `owner` (optional): Repository owner (defaults to `jaegertracing` or `GITHUB_REPOSITORY` env var)
- `repo` (optional): Repository name (defaults to `jaeger` or `GITHUB_REPOSITORY` env var)

### Examples

**Process quota for a specific user in the default repository:**
```bash
node pr-quota-manager.js newcontributor
```

**Process quota for a user in a different repository:**
```bash
node pr-quota-manager.js username myorg myrepo
```

**Using environment variables for repository:**
```bash
export GITHUB_REPOSITORY="jaegertracing/jaeger"
node pr-quota-manager.js contributor
```

### Dry-Run Mode

Test the script without making any actual changes:

```bash
# Using flag
node pr-quota-manager.js username --dry-run

# Using environment variable
DRY_RUN=true node pr-quota-manager.js username
```

In dry-run mode, the script will:
- Show exactly what actions it would take
- Not create/modify labels
- Not post comments
- Not make any API modifications
- Still fetch and analyze PRs for accurate simulation

## Listing Open PRs by Author

Use the utility script to see all open PRs grouped by author:

```bash
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

The script will display:

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

📋 Summary:
  - Blocked: 2 PRs
  - Unblocked: 0 PRs
  - Unchanged: 1 PRs
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

## Troubleshooting

### "GITHUB_TOKEN environment variable is required"

Make sure you've set the `GITHUB_TOKEN` environment variable:
```bash
export GITHUB_TOKEN="your_token_here"
```

### "403 Forbidden" errors

Your GitHub token may not have the required permissions. Ensure it has:
- `repo` scope for private repositories
- `public_repo` scope for public repositories

### "Cannot find module '@octokit/rest'"

Install the required dependency:
```bash
cd .github/scripts
npm install @octokit/rest
```

### API Rate Limiting

GitHub has rate limits for API requests:
- Authenticated requests: 5,000 requests per hour
- The script makes approximately 2-5 API calls per user

If you hit rate limits, wait for the limit to reset or use a different token.

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

`.github/workflows/pr-quota-manager.yml` calls the action in this directory on:
- Pull request opened, closed, reopened, or pushed to
- Manual workflow dispatch, which is also how you process a single user without a command line — pass `username`, and `dryRun` if you only want to see what would happen

The action runs the script through `actions/github-script` with the repository's built-in `GITHUB_TOKEN`. The `pull_request_target` trigger runs in the base repository context, so that token carries the workflow's declared write permissions even for pull requests from forks.

`jaegertracing/jaeger-ui` calls the same action at a pinned SHA rather than keeping its own copy of the script.

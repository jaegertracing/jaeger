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
| `per-issue-limit` | no | `false` | Reconcile the complete open-PR graph so each linked same-repository issue has one active PR. |
| `issue-number` | no | — | For an issue-label event, limit comment reconciliation to PRs linked to this issue. |
| `exempt-users` | no | empty | Comma-separated logins that skip the author quota. Case-insensitive. The quota workflow passes the `QUOTA_EXEMPT_USERS` repository variable. |

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

1. **History Audit**: Merged PR count across `jaegertracing/jaeger` and `jaegertracing/jaeger-ui`, and whether the login is exempt
2. **Current Stats**: Merged count, calculated quota (`unlimited` when there is no cap), and open PR count in the current repository
3. **Processing Actions**: Each open PR being blocked or unblocked, and each closed PR losing `pr-quota-reached`
4. **Summary**: Total counts of blocked, unblocked, unchanged, and closed-unlabeled PRs

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
  - Closed unlabeled: 0 PRs
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

Merged pull requests are those the user opened that were merged in `jaegertracing/jaeger` or `jaegertracing/jaeger-ui`. The count is the Search API `total_count` for `is:pr is:merged author:<login>` across both repositories, so a squash merge counts. The open cap applies only to the repository running the action. A login listed in `QUOTA_EXEMPT_USERS` has no cap. Three or more merged pull requests is unlimited: the action does not add `pr-quota-reached`.

| Merged PRs in jaeger and jaeger-ui | Open PRs allowed in the current repository |
|-----------|-------|
| 0 | 1 |
| 1 | 2 |
| 2 | 3 |
| 3+ | Unlimited |

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
- Authenticated REST requests: 5,000 requests per hour
- The Search API is a separate, smaller limit
- For one user the script lists that user's open pull requests in the current repository and runs two searches: merged pull requests in `jaegertracing/jaeger` and `jaegertracing/jaeger-ui`, then closed pull requests in the current repository that still have `pr-quota-reached`

A failed or incomplete search fails the run. The script does not fall back to counting one repository. If you hit rate limits, wait for the limit to reset or use a different token.

## How It Works

1. **Lists open PRs** by the author in the current repository, oldest first
2. **Counts merged PRs** the author opened in `jaegertracing/jaeger` and `jaegertracing/jaeger-ui` via the Search API. A failed or incomplete search fails the run before any label is written
3. **Finds closed PRs** in the current repository that still have `pr-quota-reached`, again via Search
4. **Calculates quota** from the combined merged count. Three or more, or a login in `QUOTA_EXEMPT_USERS`, is unlimited
5. **Applies labels** to the open PRs:
   - Within quota: remove `pr-quota-reached` when it is present
   - Over quota: add `pr-quota-reached`
   - Unlimited: remove the label from every open PR, and do not add it
6. **Removes `pr-quota-reached` from those closed PRs** without commenting
7. **Posts comments** on open PRs only, and only when an open PR changes state:
   - Blocking comment when a PR is first blocked
   - Unblocking comment when a PR moves into the active queue

## Integration with GitHub Actions

`.github/workflows/pr-quota-manager.yml` calls the action for the existing
author quota on pull request opened, closed, reopened, or pushed events, and on
manual workflow dispatch. Manual runs pass `username`, and optionally `dryRun`.
The workflow passes the `QUOTA_EXEMPT_USERS` repository variable through the
`exempt-users` input. `jaegertracing/jaeger-ui` uses a pinned copy of this
action, so it keeps the previous behavior until that pin is updated and its
workflow passes the same variable.

`.github/workflows/pr-per-issue-manager.yml` separately enables the per-issue
policy on pull request lifecycle events, the `allow-multiple-prs` issue-label
override, and a daily sweep. The stale workflow uses `GITHUB_TOKEN`, so its label
changes cannot trigger another workflow; the daily sweep observes those changes.

The action runs the script through `actions/github-script` with the repository's built-in `GITHUB_TOKEN`. The `pull_request_target` trigger runs in the base repository context, so that token carries the workflow's declared write permissions even for pull requests from forks.

`jaegertracing/jaeger-ui` calls the same action at a pinned SHA rather than keeping its own copy of the script.

## Per-issue active PR limit

Jaeger enables `per-issue-limit` for its own workflow. The action reads GitHub's
canonical `closingIssuesReferences` relationship and considers only linked issues
in the same repository. An open PR with the `stale` label is inactive, matching
the stale workflow's policy; drafts and PRs from maintainers remain active.

For each linked issue, the oldest active PR (with PR number as the tie-breaker)
is primary. Draft and reopened PRs keep their original creation time, so either
can remain primary. The action labels the other active PRs `duplicate` and
maintains one marker comment on each naming its primary PR. A maintainer can
exempt an umbrella issue or intentionally competing implementations by applying
`allow-multiple-prs` to the issue. The policy workflow serializes repository runs
separately from the author-quota workflow. It reads the complete, paginated graph
of open PRs and their canonical links before mutations, so a PR linked to
multiple issues keeps its full duplicate state. The query bounds nested
connections and fails safely if a PR has more than 20 labels, more than 10 linked
issues, or a linked issue has more than 10 labels; it never silently misses
`stale`, `duplicate`, or `allow-multiple-prs`. The daily sweep reconciles a
remaining PR when another PR removes its issue link. Only comments authored by
`github-actions[bot]` with the
policy marker control label cleanup. Resolved comments retain the policy marker
but drop the separate label-ownership marker, allowing the action to reuse its
one maintained comment without claiming a later human-applied `duplicate`
label. The ownership marker records whether the action added the label, so an
existing manual `duplicate` label is never claimed or removed. Scheduled and
manual sweeps scan all open PR comments for policy markers, which lets a later
run repair a partial comment/label write. PR and issue-label events inspect only
their affected PRs, PRs already carrying `duplicate`, and newly calculated
duplicates. Cleanup removes labels while the ownership marker is still present
and restores removed labels if a later removal fails. It does not mutate stale
PRs, avoiding stale-policy reactivation caused by bot activity.

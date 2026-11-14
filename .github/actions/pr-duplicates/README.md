# Duplicate PRs

Allows one active pull request per linked issue, by labelling the other pull requests that close the same issue with `duplicate` and naming the primary pull request in a comment.

The logic lives in [`.github/scripts/pr-duplicates.js`](../../scripts/pr-duplicates.js), next to the dependencies and jest setup that its tests need. This directory holds the action wrapper and this guide.

## Using the action

```yaml
permissions:
  pull-requests: write
  issues: write

steps:
  - uses: actions/checkout@v7
  - uses: ./.github/actions/pr-duplicates
    with:
      token: ${{ github.token }}
```

| Input | Required | Default | Purpose |
|---|---|---|---|
| `token` | yes | — | Token used to label and comment. Needs `issues: write` and `pull-requests: write`. |
| `pull-request-number` | no | — | The PR whose event asked for the run. Its comments, and those of PRs that share a linked issue with it, are checked in addition to those of every duplicate. |
| `issue-number` | no | — | The issue whose label event asked for the run. The comments of the PRs linked to it are checked in addition to those of every duplicate. Ignored when `pull-request-number` is set. |
| `dry-run` | no | `false` | Log the intended labels and comments without writing anything. |

Without `pull-request-number` and `issue-number` the action checks the comments of every open PR.

`.github/workflows/pr-duplicates.yml` calls the action on pull request lifecycle
events, on the `allow-multiple-prs` issue-label override, and in a daily sweep.
The stale workflow uses `GITHUB_TOKEN`, so its label changes cannot trigger
another workflow; the daily sweep observes those changes.

## The policy

The action reads GitHub's
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

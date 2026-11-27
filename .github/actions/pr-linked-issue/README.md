# PR Linked Issue

This action requires a pull request from an outside contributor to be linked to an open issue that carries the `help wanted` label. It flags a pull request that has no such issue: it adds one of the two flag labels, `missing-issue` or `unapproved-issue`, and explains the policy in a comment. A scheduled run with `close-expired-pull-requests` closes the pull request a week later. It is a composite action so that another Jaeger repository can call it at a pinned commit.

The logic lives in [`pr-linked-issue.js`](pr-linked-issue.js), next to its tests, which run through `npm test` in [`.github/scripts`](../../scripts).

## Using the action

```yaml
permissions:
  pull-requests: write
  issues: write

steps:
  - uses: jaegertracing/jaeger/.github/actions/pr-linked-issue@<commit-sha>
    with:
      token: ${{ github.token }}
      pull-request-number: ${{ github.event.pull_request.number }}
```

| Input | Required | Default | Purpose |
|---|---|---|---|
| `token` | yes | — | Token used to label, comment and close. Needs `issues: write` and `pull-requests: write`. |
| `pull-request-number` | no | — | The pull request to check. Takes precedence over `issue-number`. |
| `issue-number` | no | — | The issue whose event asked for the run. The open pull requests linked to it are checked. |
| `close-expired-pull-requests` | no | `false` | Check only the pull requests that carry a flag label, and close each one that still violates the policy a week after it was flagged. |
| `dry-run` | no | `false` | Report which pull requests violate the policy without writing anything. |

Without `pull-request-number` and `issue-number` the action checks every open pull request.

## The policy

A pull request meets the policy when one of the first 20 issues that it closes is open, carries `help wanted`, and belongs to a repository of the same organization. The action does not check who applied `help wanted`, because GitHub lets only a user with triage permission or higher label an issue.

The flag label says what the pull request waits for:

| Label | The pull request | What it waits for |
|---|---|---|
| `unapproved-issue` | is linked to an open issue of the organization that does not carry `help wanted` | the author asks on the issue, and a maintainer decides whether the issue gets `help wanted` |
| `missing-issue` | is linked to no issue, or only to closed issues and issues of another organization | the author links an issue |

After a complete run a flagged pull request carries one flag label. When the linked issues of the pull request change, the action adds the label that now applies before it removes the other.

A pull request is exempt when its author has write access to the repository, when it comes from Dependabot or Renovate, or when a maintainer applied the `no-issue-needed` label to it.

When a flagged pull request later meets the policy, the action removes its flag label.

## Closing a flagged pull request

A flagged pull request is expired when it still violates the policy and its policy comment is at least seven days old. A run with `close-expired-pull-requests` checks the pull requests that carry a flag label. It removes the label from one that now meets the policy, and it closes an expired one. The run flags no pull request, so the seven days always start at a check that an event started, which posts the policy comment. A pull request that carries a flag label and has no policy comment, for example because a person applied the label by hand, is not closed.

The seven days count from the day the pull request was flagged. A comment or a push does not extend them, and neither does a change of the linked issues that leaves the pull request flagged, because the action rewrites the policy comment in place. A pull request that meets the policy and is flagged again gets a new comment and a new seven days.

Two labels keep a flagged pull request open:

| Label | Effect |
|---|---|
| `no-issue-needed` | The pull request is exempt from the policy, and the action removes its flag label. |
| `do-not-expire` | The pull request stays flagged and open. Its policy comment still announces the closing. |

A closed pull request keeps its flag label. Its author links an issue in the description and reopens it, and the reopening starts a check that removes the label. A pull request that is reopened while it still violates the policy is closed again by the next scheduled run.

The stale workflow applies its own idle timer to a flagged pull request, independently of this action. A repository that calls this action needs a scheduled job that calls it with `close-expired-pull-requests`.

## When the action runs

`.github/workflows/pr-linked-issue.yml` runs once a day with `close-expired-pull-requests`, and reacts when a pull request is opened, edited or reopened, when `no-issue-needed` is added to or removed from a pull request, and when an issue is closed, reopened, or has `help wanted` added or removed. The run that a pull request or issue event starts does not call the action. It dispatches the workflow on the default branch once for each pull request to check, and the dispatched run calls the action. For a pull request event that is the pull request. For an issue event it is each open pull request of this repository linked to the issue.

All dispatched runs for one pull request share a concurrency group, so two events never write to the same pull request at once. A concurrency group holds one waiting run, so GitHub cancels a waiting run when a newer one arrives, and the newer one checks the pull request. The cancelled run is on the default branch and does not appear among the checks of the pull request. The cost is that a check that fails also shows only as a failed run on the default branch; the check on the pull request reports whether the dispatch succeeded. The scheduled run of the workflow checks only the pull requests that are already flagged, so a pull request that was open before the workflow existed is flagged only when one of those events touches it. The scheduled run is outside the concurrency groups, so it can write to a pull request at the same time as a check that an event started. Each of the two acts on the labels that it read at its start.

Linking an issue through the sidebar, and a label or state change on a linked issue in another repository, fire no event for the pull request. A flagged pull request is checked again by the next scheduled run. A pull request that is not flagged is checked again when its description is edited, or when the workflow is run manually for it.

## The comment

The action keeps one current comment on a flagged pull request. It rewrites that comment when the linked issues change, and when the pull request meets the policy. A pull request that is flagged again gets a new comment, because GitHub notifies the author of a new comment and not of an edited one.

The action recognizes its comment by a hidden marker in the comment body. It honors the marker only in a comment from `github-actions[bot]`, which is the account of a workflow, or from an owner, organization member or collaborator. Pasting the marker into any other comment does not make the action rewrite that comment.

## Running locally

The script in `.github/scripts` applies the policy to every open pull request of a repository, which is how the policy reaches the pull requests that were open before the workflow existed. It reports what it would do and writes nothing unless `--apply` is given:

```bash
cd .github/scripts
npm ci
export GITHUB_TOKEN=$(gh auth token)
node apply-pr-linked-issue-policy.js                          # dry run on jaegertracing/jaeger
node apply-pr-linked-issue-policy.js jaegertracing jaeger-ui  # dry run on another repository
node apply-pr-linked-issue-policy.js --pr 1234                # dry run on one pull request
node apply-pr-linked-issue-policy.js --apply                  # label and comment
node apply-pr-linked-issue-policy.js --close-expired-pull-requests          # dry run of the scheduled closing
```

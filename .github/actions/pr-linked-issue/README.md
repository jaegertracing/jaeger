# PR Linked Issue

This action requires a pull request from an outside contributor to be linked to an open issue that carries the `help wanted` label. It adds the `missing-issue` and `stale` labels to a pull request that has no such issue, and explains the policy in a comment. It is a composite action so that another Jaeger repository can call it at a pinned commit.

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
| `token` | yes | — | Token used to label and comment. Needs `issues: write` and `pull-requests: write`. |
| `pull-request-number` | no | — | The pull request to check. Every open pull request is checked when it is empty. |
| `dry-run` | no | `false` | Report which pull requests violate the policy without writing anything. |

## The policy

A pull request meets the policy when one of the first 20 issues that it closes is open, carries `help wanted`, and belongs to a repository of the same organization. The action does not check who applied `help wanted`, because GitHub lets only a user with triage permission or higher label an issue.

A pull request is exempt when its author has write access to the repository, when it comes from Dependabot or Renovate, or when a maintainer applied the `no-issue-needed` label to it.

The action closes no pull request. The stale workflow of the repository closes a pull request that keeps the `stale` label. When the pull request later meets the policy, the action removes both labels.

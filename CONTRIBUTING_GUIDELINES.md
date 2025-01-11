# How to Contribute to Jaeger

We'd love your help!

Jaeger is [Apache 2.0 licensed](./LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

Table of Contents:

* [Making a Change](#making-a-change)
* [License](#license)
* [Certificate of Origin - Sign your work](#certificate-of-origin---sign-your-work)
* [Branches](#branches)

## Making a Change

### Open an issue first

**Before making any significant changes, please open an issue**. Each issue
should describe the following:

* Requirement - what kind of business use case are you trying to solve?
* Problem - what in Jaeger blocks you from solving the requirement?
* Proposal - what changes do you propose to solve the problem or improve the existing situation?
* Any open questions to address

Discussing your proposed changes ahead of time will make the contribution
process smooth for everyone. Once the approach is agreed upon, make your changes
and open a pull request (PR).

### Assigning Issues

We do not assign issues to contributors. It is almost never the case that multiple
people jump on the same issue, and practice showed that occasionally people who ask
for an issue to be assigned to them later have a change in priorities and are unable
to find time to finish it, which leaves the issue in limbo. 
So if you have a desire to work on an issue, feel free to mention it in the comment and just submit a PR.

### Creating a pull request

If you are new to GitHub's contribution workflow, we recommend the following setup:
  * Go to the respective Jaeger repo on GitHub and create a fork using the button at the top. Select a destination org where you have write permissions (usually it is your personal "org").
  * Clone the fork into your workspace.
  * (Recommended): register upstream repo as remote
    * After you clone your forked repo, running below command
      ```bash
      git remote -v
      ``` 
      Will show `origin`, e.g. 
      ```origin	git@github.com:{username}/jaeger.git```
    * Add `upstream` remote:
      ```bash
      git remote add upstream git@github.com:jaegertracing/jaeger.git
      ```
    * Fetch it:
      ```bash
      git fetch upstream main
      ```
    * Repoint your main branch: 
      ```bash
      git branch --set-upstream-to=upstream/main main
      ```
    * With this setup, you will not need to keep your main branch in the fork in sync with the upstream repo.

Once you're ready to make changes:
  * Create a new local branch (DO NOT make changes to `main`, it will cause CI errors).
  * Commit your changes, making sure **each commit is signed**, e.g.
    ([see below](#certificate-of-origin---sign-your-work)).
    ```bash
    git commit -s -m "Your commit message"
    ``` 
  * You do not need to squash the commits, it will happen once the PR is merged into the official repo (but each individual commit must be signed).
  * When satisfied, push the changes. Git will likely ask for upstream destination, so you push commits like this:
    ```bash
    git push --set-upstream origin {branch-name}
    ```
  * After you push, look for the output, it usually contains a URL to create a pull request.

Each PR should have:

* A descriptive title, known as ["commit message"][good-commit-msg]. In summary:
  * Limit the title to 50 characters
  * Capitalize the title
  * Do not end the title with a period
  * Use the imperative mood in the title
* A description of the problem it is solving. It could be simply a reference to the corresponding issue, e.g. `Resolves #123`.
* A summary of changes made to solve the problem. Explain _what_ and _why_ instead of _how_.

## License

By contributing your code, you agree to license your contribution under the
terms of the [Apache License](./LICENSE).

If you are adding a new file it should have a header like below. In some
languages, e.g. Python, you may need to change the comments to start with `#`.
The easiest way is to copy the header from one of the existing source files and
make sure the year is current and the copyright says "The Jaeger Authors".

```
// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```

## Certificate of Origin - Sign your work

By contributing to this project you agree to the
[Developer Certificate of Origin](https://developercertificate.org/) (or simply
[DCO](./DCO)). This document was created by the Linux Kernel community and is a
simple statement that you, as a contributor, have the legal right to make the
contribution.

The sign-off is a simple line at the end of the explanation for the patch, which
certifies that you wrote it or otherwise have the right to pass it on as an
open-source patch. The rules are pretty simple: if you can certify the
conditions in the [DCO](./DCO), then just add a line to every git commit
message:

    Signed-off-by: Bender Bending Rodriguez <bender.is.great@gmail.com>

using your real name (sorry, no pseudonyms or anonymous contributions.) You can
add the sign off when creating the git commit via `git commit -s`.

### Missing sign-offs

Note that **every commit in the pull request must be signed**. Jaeger
repositories are configured with a [DCO-bot][dco-bot] that will check sign-offs
on every commit and block the PR from being merged if some commits are missing
sign-offs. If you only have one commit or the latest commit in the PR is missing
a sign-off, the simplest way to fix this is to run:

```
git commit --amend -s
```

which will prompt you to edit the commit message while adding a signature.
Simply accept the text as is, and push the branch:

```
git push --force
```

If some commit in the middle of your commit history is missing the sign-off, the
simplest solution is to squash the commits into one and sign it. For example,
suppose that your branch history looks like this:

```
fe43631 - Fix HotROD Docker command
933efb3 - Add files for ingester
214c133 - Rename gas to gosec
0a40309 - Update Makefile build_ui target to lerna structure
7919cd9 - Add support for Cassandra reconnect interval
a0dc40e - Fix deploy step
77a0573 - (tag: v1.6.0) Prepare release 1.6.0
```

Let's assume that the first commit `77a0573` was the commit before you started
work on your PR, and commits from `a0dc40e` to `fe43631` are your changes that
you want to squash. You can run the soft reset command:

```
git reset --soft 77a0573
```

It will undo all changes after commit `77a0573` and stage them. You can commit
them all at once while adding the signature:

```
git commit -s -m 'your commit message, e.g. the PR title'
```

Then push the branch:

```
git push --force
```

[good-commit-msg]: https://chris.beams.io/posts/git-commit/
[dco-bot]: https://github.com/probot/dco#how-it-works

## Branches

Upstream repository should contain only maintenance branches (e.g. `release-1.0`). For feature
branches use forked repository.

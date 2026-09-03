# Jaeger AI Usage Policy

This policy describes how the Jaeger maintainers expect contributors to use AI tools — LLMs, coding agents, autocomplete, and anything similar — when contributing to the project. It applies to every repository in the [jaegertracing](https://github.com/jaegertracing) GitHub organization and to all project spaces, including issues, pull requests, and Slack.

Jaeger is a CNCF project, so the Linux Foundation's [Generative AI Tools policy](https://www.linuxfoundation.org/legal/generative-ai) also applies to every contribution.

## Goals

This policy exists to:

- **Keep the effort balanced** – Before AI, contributors did most of the work (writing, testing, understanding). We want to keep it that way. AI should help you, not replace your effort.
- **Protect maintainer time** – Large, low-quality AI-generated PRs shift the burden to reviewers. We want to avoid that.
- **Ensure understanding** – Contributors should understand and be able to explain every change they submit.
- **Keep conversations human** – Code review is a discussion between people. A tool can help you write, but the positions have to be yours.

## Good use of AI

We use AI tools ourselves and assume most contributors do too. Helpful uses include:

- Using AI to help you understand the code.
- Using AI to write drafts of code, tests, or docs.
- Using AI to explore ideas or try different approaches.
- Using AI to help you word an issue, a PR description, or a reply to review feedback, as long as you read it, correct what you disagree with, and stand behind what you post.

## Disallowed use of AI

- Copy-pasting AI output without reading or understanding it.
- Submitting AI-generated code without testing.
- Letting an agent answer review feedback autonomously, posting its replies without reading and approving each one. What matters is not whether a tool wrote the words, but whether you verified them and hold the position they take.
- Pointing an agent at someone else's pull request and posting what it produces. We already run automated review tools; we do not need more generated commentary. Using AI to help you understand a change you are reviewing is fine, as long as the review you post is your own.
- Submitting a change you cannot explain, contextualize, or justify during review.

## Your responsibility

- You own everything you submit, even if AI wrote it.
- You must understand your code well enough to explain it. "My agent wrote that" is not an answer to a review comment.
- You must run tests locally before opening or updating a PR.
- Your DCO sign-off covers AI output the same as code you typed. A generator can reproduce code from its training data, and you cannot inspect where its output came from, so keep it to code you are confident is yours to contribute under Jaeger's license. If you are not confident, do not submit it.
- Declare AI usage with the checkbox in the pull request template. Keep it out of commit trailers, though: a `Co-authored-by` line for your agent distorts contributor statistics.

## Talk to us yourself

We want to engage with the human behind the contribution. Tell us your own reasoning: why you framed the problem this way, what you tried, what you are unsure about, and what your use case needs. If the text reads as though an agent wrote it, it is much harder for us to find the person we are supposed to be collaborating with.

Nobody can tell from the outside whether you read a reply before posting it, so we judge review conversations by what follows from them: whether a reply engages with the point the reviewer actually raised rather than restating the code, whether its claims survive checking (a reply asserting a concern is addressed when the diff says otherwise is the common failure), and whether you can hold the position in the next round. Replies that fail that test cost the reviewer another cycle for nothing, and enough of them are grounds for us to stop reviewing the PR.

Match the level of detail to the change. AI tools happily generate walls of text that bury the actual problem, and a long PR description is not evidence of a well-considered change. The same applies to code comments: an agent tends to write for the reviewer, narrating rejected alternatives and arguing that the change is correct. That belongs in the PR description, not in the codebase, where it becomes clutter once the PR is merged. Comments should explain what a future reader cannot derive from the code itself.

## Maintainer time is limited

AI has made opening a pull request nearly free, while reviewing one costs a maintainer just as much as it always did. So bias towards discussion first — for anything beyond a clearly scoped fix, [open an issue](./CONTRIBUTING_GUIDELINES.md#open-an-issue-first) and agree on the direction before you implement it — and keep each change small and focused. New contributors are also subject to a cap on simultaneous open pull requests, described in [Pull Request Limits for New Contributors](./CONTRIBUTING_GUIDELINES.md#pull-request-limits-for-new-contributors).

## Enforcement

- PRs that look like low-effort AI slop will be closed, and we may close them without writing a detailed technical critique first.
- Repeated violators may be banned from the project.

None of this is aimed at contributors who want to learn the project and engage with us honestly — that is exactly what we are hoping for. If you are new and trying to understand something, ask, and we will help.

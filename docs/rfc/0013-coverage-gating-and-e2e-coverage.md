# RFC 0013: Reliable Coverage Gating and Real E2E Coverage

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-28
- **Last Updated:** 2026-07-28
- **Issue:** [#9084](https://github.com/jaegertracing/jaeger/issues/9084)
- **Related:** [ADR-004 Migrating Coverage Gating to GitHub Actions](../adr/004-migrating-coverage-gating-to-github-actions.md) · [#1516](https://github.com/jaegertracing/jaeger/issues/1516) · PRs [#4964](https://github.com/jaegertracing/jaeger/pull/4964), [#5811](https://github.com/jaegertracing/jaeger/pull/5811), [#5836](https://github.com/jaegertracing/jaeger/pull/5836), [#9086](https://github.com/jaegertracing/jaeger/pull/9086), [#9088](https://github.com/jaegertracing/jaeger/pull/9088)

---

## Implementation status

| Milestone | Scope | Status |
| --- | --- | --- |
| M1 | Unstick PRs: realign `after_n_builds` with the real upload count | ⬜ |
| M2 | Enforce the upload invariant: every upload carries coverage, count matches | ⬜ |
| M3 | Graceful shutdown of the e2e jaeger binary | ⬜ |
| M4 | Spike: `-cover` + `GOCOVERDIR` on one e2e leg, measure the gain | ⬜ |
| M5 | Roll binary coverage across the e2e matrix; remove the double compile | ⬜ |
| M6 | Diff-level gating in `Coverage Gate` as defense in depth | ⬜ |
| M7 | Record the outcome as an ADR | ⬜ |

---

## Abstract

Jaeger measures code coverage in 27 separate CI uploads and gates pull requests on it through three coverage-related required status checks — `codecov/patch`, `codecov/project`, and `Coverage Gate`. Two independent defects in that arrangement have compounded into a week of pull requests that cannot merge.

The first is a **fidelity** defect. Of the 27 coverage uploads, only 12 measure anything. The other 15 come from the `e2e` matrix cells, which exercise jaeger as a **separate OS process** that is compiled without coverage instrumentation, so their in-process `go test -coverpkg=./...` profiles recorded the test harness and a module-wide list of zeros — never the storage code the cells exist to exercise. [#9084](https://github.com/jaegertracing/jaeger/issues/9084) established this; [#9086](https://github.com/jaegertracing/jaeger/pull/9086) then removed the instrumentation as waste, correctly, and measured coverage moved by 0.03 percentage points.

The second is a **gating** defect, which the first one exposed. `.codecov.yml` sets `codecov.notify.after_n_builds: 18`, a hand-maintained mirror of the CI job count that withholds *every* Codecov notification until 18 uploads arrive. Once the 15 phantom profiles stopped registering, the count fell to 12, Codecov silently stopped posting `codecov/patch` and `codecov/project` — both **required** status checks on `main` — and every pull request since 2026-07-23 has been stuck awaiting checks that will never appear. The threshold had been calibrated against uploads that carried no signal: it counts *uploads*, not *coverage*.

[ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md) moved coverage gating into a `Coverage Gate` check computed by the `ci-summary-report.yml` fan-in, intending Codecov to become a trending tool. It replaced only the **project-level** gate, though: it specified an absolute floor and a no-regression comparison, both computed from a single total-coverage percentage, and never specified a diff-level gate. So `codecov/patch` remains the only check enforcing the 95% patch target that `AGENTS.md` promises contributors, and it carries a signal the project-level gate structurally cannot reproduce — a pull request can leave a substantial diff uncovered without moving the project total enough to trip a threshold 2.4 points below it.

This RFC therefore proposes to **keep Codecov as a gate and repair the fragility that broke it**, not to retire it. The immediate fix is a one-line realignment of `after_n_builds` to the real upload count. The durable fix is to enforce the invariant that the threshold was always assuming — that every job which uploads coverage actually contributes coverage, and the threshold equals that number — in CI, so a violation fails loudly instead of silently freezing the merge queue. A signal-free upload is treated as the defect it is, not as a quantity to subtract from the threshold. Separately, this RFC proposes to make e2e coverage real by building the spawned binary with `go build -cover` and collecting `GOCOVERDIR` output — which requires first replacing the harness's `SIGKILL` shutdown, since a `SIGKILL`ed Go process flushes no coverage counters at all. Diff-level gating in the fan-in is proposed last, as defense in depth against a Codecov outage rather than as a precondition for removing anything.

---

## 1. Motivation

### 1.1 What is broken right now

[PR #9091](https://github.com/jaegertracing/jaeger/pull/9091) is a one-line dependency bump. All 60-odd of its CI checks pass, including `All CI Checks Passed` and `Coverage Gate`. It cannot merge, because branch protection on `main` requires six contexts and two of them never appear:

```
codecov/patch          ← never posted
codecov/project        ← never posted
check-label            ✔
All CI Checks Passed   ✔
Coverage Gate          ✔
Metrics Comparison     ✔
```

Codecov received the uploads — the CLI logs report `Your upload is now queued for processing` and the merged report on Codecov's side is `state: complete` with 97.44% coverage. It simply never notified. The last pull request to receive a `codecov/patch` check run was [#9088](https://github.com/jaegertracing/jaeger/pull/9088), merged 2026-07-23 15:47Z, under two hours before [#9086](https://github.com/jaegertracing/jaeger/pull/9086) landed. Every pull request merged since has gone in without those checks, which requires an administrator to bypass branch protection on each one.

A second-order consequence: because `codecov/patch` is the only diff-level coverage gate in the system (§3.3), pull requests merged during the outage window were checked against the project-wide floor alone, which sits 2.4 points below the current total. The window is five days, so this is a small, bounded gap rather than an established practice — but it is a reason to restore the check promptly, and an argument against treating its absence as tolerable.

### 1.2 The measurement nobody was getting

[#9084](https://github.com/jaegertracing/jaeger/issues/9084) profiled an `opensearch 3.x e2e` job and found roughly four of its six minutes were compilation, much of it avoidable. Its third finding is the one that matters here:

> The e2e harness launches jaeger as a **separate OS process** (`cmd/jaeger/internal/integration` `binary.go` starts it and polls `:13133/status`), and that binary is built **without** `-cover`. In-process `go test -coverpkg=./...` coverage therefore cannot capture the storage code actually exercised in the child process — it mostly reflects the test harness. The whole-module instrumentation compile cost buys coverage that doesn't measure the e2e path.
>
> **Proposed fix:** drop or narrow `-coverpkg` for the `jaeger-v2-storage-integration-test` target, **or build the spawned binary with `-cover` + `GOCOVERDIR` if e2e coverage is actually wanted.**

[#9086](https://github.com/jaegertracing/jaeger/pull/9086) took the first branch of that fix. The second branch — the one that would actually produce e2e coverage — remains undone, and is the substance of §5.

### 1.3 A threshold that has drifted three times

`after_n_builds` is a count of coverage uploads that Codecov waits for before sending any notification. Its history is a record of manual resynchronization against a moving CI matrix:

| When | Value | Why |
| --- | --- | --- |
| 2018-06-01 ([#857](https://github.com/jaegertracing/jaeger/pull/857)) | *absent* | Only unit tests uploaded coverage — one report, nothing to wait for. |
| 2023-11-25 ([#4964](https://github.com/jaegertracing/jaeger/pull/4964)) | `11` | Same PR that began collecting coverage from integration tests ([#1516](https://github.com/jaegertracing/jaeger/issues/1516)). With 11 jobs uploading independently, Codecov would otherwise notify off the first arrival and report a partial merged report. |
| 2024-08-06 ([#5811](https://github.com/jaegertracing/jaeger/pull/5811)) | `19` | The value had drifted below the real upload count, producing "false positives … a comment about non-covered code which is actually covered, but its result uploads came after the 11 other results." |
| 2024-08-14 ([#5836](https://github.com/jaegertracing/jaeger/pull/5836)) | `18` | Adjusted inside an unrelated dependency bump. |
| 2026-07-23 ([#9086](https://github.com/jaegertracing/jaeger/pull/9086)) | `18` | Real contributing uploads fell to 12. Value untouched. |

The setting is load-bearing for the correctness of `codecov/patch`: too low, and a diff whose covering upload arrives late is reported as uncovered. But it is a hand-maintained mirror of the job matrix, and it fails asymmetrically. Drift **low** produces loud false positives, caught in a day in 2024. Drift **high** produces silence — Codecov waits forever, posts nothing, and branch protection blocks every merge with no diagnostic anywhere in the CI output. That asymmetry is why a one-line change to a Makefile froze the repository for a week.

---

## 2. Current state

### 2.1 Coverage upload inventory

27 uploads reach Codecov per full CI run, from 12 workflows, each tagged with a distinct flag by [`.github/actions/upload-codecov`](../../.github/actions/upload-codecov/action.yml). They divide cleanly by which make target runs them:

| Legs | Make target | Process model | Uploads | Real coverage |
| --- | --- | --- | --- | --- |
| unit tests | `make test` | in-process | 1 | 🟢 |
| badger, grpc, clickhouse `direct` | `storage-integration-test` | in-process | 3 | 🟢 |
| cassandra `direct` (4.x, 5.x) | `storage-integration-test` | in-process | 2 | 🟢 |
| elasticsearch `direct` (7.x, 8.x, 9.x) | `storage-integration-test` | in-process | 3 | 🟢 |
| opensearch `direct` (1.x, 2.x, 3.x) | `storage-integration-test` | in-process | 3 | 🟢 |
| all `e2e` cells, kafka, memory-v2, query, tailsampling | `jaeger-v2-storage-integration-test` | **spawned binary** | 15 | 🔴 |
| | | | **27** | **12** |

`storage-integration-test` retains `-coverpkg=./...` and runs the storage code in the test process, so its profiles are meaningful. `jaeger-v2-storage-integration-test` lost `-coverpkg` in [#9086](https://github.com/jaegertracing/jaeger/pull/9086); its `JAEGER_V2_STORAGE_PKGS` is `./cmd/jaeger/internal/integration`, which `.codecov.yml` lists under `ignore`. Those 15 profiles now contain nothing Codecov will count, so it drops the sessions.

The arithmetic is exact, and confirmed against Codecov's API across the boundary: 27 sessions on every commit through `46e28f890`, 12 on every commit from `75ff3d6fe` ([#9086](https://github.com/jaegertracing/jaeger/pull/9086)) onward. #9086's own PR run already reported 12, since the change was live on its branch.

### 2.2 Two gates over the same data

Both gating systems consume the same per-job coverage profiles, by different routes:

```
                                       ┌─▶ codecov CLI ──▶ Codecov ──▶ codecov/patch, codecov/project
each CI job writes cover.out ──────────┤                               (gated by after_n_builds: 18)
                                       └─▶ coverage-<flag> artifact ──▶ ci-summary-report.yml
                                                                        gocovmerge + filter_coverage.py
                                                                        ──▶ Coverage Gate
```

The fan-in path is ADR-004's, and it is immune to the drift in §1.3 by construction: it merges whatever `coverage-*` artifacts the run produced and has no notion of an expected count. It is *not* immune to the fidelity problem in §1.2 — it merges the same 15 phantom profiles, so improving them (M4, M5) improves `Coverage Gate` as much as it improves Codecov.

### 2.3 ADR-004's unfinished follow-ups

[ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md) is tagged *Accepted (implemented)*, and its own implementation is complete: the fan-in exists, merges coverage, applies `.codecov.yml`'s exclusions through [`scripts/e2e/filter_coverage.py`](../../scripts/e2e/filter_coverage.py), and always posts a `Coverage Gate` check-run. What was never done is everything *around* it, and the current outage is the direct result.

1. **`Coverage Gate` does not implement a diff gate, so the migration was never completable as scoped.** ADR-004's Requirement 2 lists exactly two gates, both derived from one project-wide percentage: an absolute 95% floor, and no-regression against the `main` baseline. There is no equivalent of Codecov's 95% *patch* target, even though `codecov/patch` was a required check at the time. The two systems are therefore not interchangeable in the direction ADR-004 assumed, and this is the substantive gap in it — not an oversight in the follow-through.
2. **Codecov was consequently never retired as a gate, and branch protection reflects reality rather than the ADR.** Requirement 4 specifies that `Coverage Gate` is always created "so it can be used as a required status check in branch protection," and it was — *in addition to* `codecov/patch` and `codecov/project`, not instead of them. ADR-004's Consequences left the rest as "Codecov remains active for long-term trending; removing it can be a follow-up decision." §4 declines that follow-up: given gap 1, Codecov is not currently redundant.
3. **The fidelity of the merged profile was never examined.** ADR-004 treats the `coverage-*` artifacts as ground truth. #9084 later established that 15 of the 27 measure the wrong process.
4. **The no-regression baseline is coupled to fidelity changes.** The baseline is a cached single number from the last `main` run. Any milestone that materially changes what is measured (M5) steps the baseline, which the gate tolerates in the upward direction but which makes a subsequent revert of that milestone fail the gate.

---

## 3. Problem analysis

### 3.1 Why Codecov went silent

`codecov.notify.after_n_builds: N` instructs Codecov to withhold **all** notifications — commit statuses, check runs, and the PR comment — until `N` uploads have been received for the commit. At 12 received against 18 expected, the notify path never runs. This is observable in Codecov's API: commits that received checks carry `ci_passed: true`, while [#9091](https://github.com/jaegertracing/jaeger/pull/9091)'s head carries `ci_passed: null`, because Codecov never got as far as evaluating CI state.

`codecov.notify.require_ci_to_pass: yes` does not compensate. It gates on the **CI build state** — Codecov queries the commit's overall state and defers while it is pending, suppressing notifications entirely if CI failed — which is a different axis from *how many coverage uploads have arrived*. A partial coverage report can sit behind a perfectly green build. It is also not a mechanism to lean on: `codecov/patch` and `codecov/project` are themselves required contexts on `main`, so Codecov's own view of "is this commit green?" is entangled with output it has not yet produced.

### 3.2 Why the e2e profiles are empty

The harness spawns jaeger and, in cleanup, kills it ([`cmd/jaeger/internal/integration/binary.go`](../../cmd/jaeger/internal/integration/binary.go)):

```go
// Stop kills the process and waits for it to exit.
func (b *Binary) Stop(t *testing.T) {
	t.Helper()
	if err := b.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("Failed to kill %s process: %v", b.Name, err)
	}
	b.Process.Wait()
}
```

`Process.Kill()` sends `SIGKILL`, which is uncatchable. Go writes coverage counters from the runtime's exit path, so a `SIGKILL`ed binary flushes **nothing** — even if it were built with `-cover` and given a `GOCOVERDIR`, the directory would come back empty. This is the load-bearing prerequisite for §5, and the most likely reason previous attempts at e2e binary coverage produced no data and were abandoned.

### 3.3 The diff-gate gap

[`AGENTS.md`](../../AGENTS.md) tells contributors:

> Codecov enforces a **95% patch target** (`.codecov.yml`), so a PR whose diff dips below it fails CI.

That is enforced by `codecov/patch` alone. `Coverage Gate` computes a single project-wide percentage from `go tool cover -func` and applies the 95% floor and the no-regression comparison to it. With the project total at 97.44%, a pull request can add a substantial amount of uncovered code without moving the total below 95%; and because the reported total carries one decimal place, small uncovered additions may not move it measurably at all.

The two gates are therefore not substitutes but complements, and they fail in opposite directions. The project gates catch slow erosion across many pull requests but are blind to a single diff that lands a poorly covered feature inside the headroom. `codecov/patch` catches exactly that, per pull request, on the changed lines — which is what makes it the check contributors actually feel, and what `AGENTS.md`'s guidance to measure patch coverage locally before pushing is written against. Any proposal that removes it owes a replacement for that signal, not an argument that the project gates approximate it.

---

## 4. Design: gate ownership

The gating question is how to stop a silent, merge-blocking failure from recurring — *without* giving up the diff-level signal that only `codecov/patch` currently provides (§3.3).

| Criteria | A: Realign `after_n_builds` to 12 | B: A, plus enforce the upload invariant | C: Retire Codecov as a gate |
| --- | --- | --- | --- |
| Unblocks PRs | 🟢 one line¹ | 🟢 one line¹ | 🟢 settings change |
| Preserves diff-level enforcement | 🟢 restores it | 🟢 restores it | 🔴 loses it outright² |
| Survives CI matrix changes | 🔴 a fourth drift is a matter of time | 🟢 violation fails CI | 🟢 nothing to drift |
| Failure mode on drift | 🔴 silent, merge-blocking | 🟢 loud, with a diagnostic | — |
| Effort | 🟢 trivial | 🟡 one fan-in step³ | 🟢 trivial |
| Third-party dependency for merging | 🔴 retained | 🔴 retained | 🟢 removed |

¹ `strict_yaml_branch: main` means the change takes effect only once on `main`, so its own pull request needs a single administrator merge. ² §3.3 — the project-level gates do not approximate it. ³ The check must assert that every upload carries coverage, not merely reconcile the threshold against however many do: #9086 left the invocation count at 27 and changed only whether 15 of them carried signal, so an invocation count would not have caught it, and a contributing-only count would have absorbed it silently.

**Recommendation: A immediately, then B.** Realigning the count restores both Codecov checks and with them the patch signal, which is the opposite trade from retiring the gate: a one-line change buys back the strongest per-pull-request coverage check in the system. B then removes the failure *class* by making the count self-verifying, which is what makes the fidelity work in §5 safe to iterate on — every future change to what the e2e legs measure will move the contributing-upload count, and that must produce a failing check with an explanatory message rather than a frozen merge queue.

Option C is rejected. Its appeal is removing the third-party dependency from the merge path, and ADR-004 anticipated it — but on the current implementation it trades a strong, diff-scoped signal for two coarse project-level ones, which is a net loss in enforcement regardless of how the outage is weighted. It becomes worth reconsidering only after M6 supplies a native diff gate, and even then the question is whether to *stop requiring* Codecov, not whether to stop uploading to it.

---

## 5. Design: measurement fidelity

Go supports coverage for whole binaries since 1.20: `go build -cover` instruments the binary, the process writes counter files into `$GOCOVERDIR` at exit, and `go tool covdata textfmt` converts them into the same text profile format that `gocovmerge`, `filter_coverage.py`, and Codecov already consume. Nothing about the existing artifact pipeline needs to change — an e2e leg would produce a profile at the same path, under the same flag, and both gating routes pick it up unchanged.

Applied to the e2e legs, this measures what those legs exist to measure: the OTel collector assembly, extension startup, configuration resolution, storage factory initialization, and shutdown paths that run only in the real binary. It is also the coverage most likely to be genuinely additive, since it is the code least reachable from unit tests — the original motivation of [#1516](https://github.com/jaegertracing/jaeger/issues/1516).

Three properties make this attractive beyond the coverage itself:

- **It subsumes [#9084](https://github.com/jaegertracing/jaeger/issues/9084) finding 2.** Today each e2e cell compiles the module twice: `(cd cmd/jaeger/ && go build .)` for the binary the harness spawns (~115s), then a whole-module instrumented test build for `go test -coverpkg` (~132s, a different build-cache key so nothing is reused). Building the binary once *with* `-cover` and leaving the test compile un-instrumented plausibly lands cheaper than the pre-#9086 state while producing real data instead of zeros.
- **Graceful shutdown is independently valuable.** Replacing `SIGKILL` with `SIGTERM` and a bounded wait exercises the collector's shutdown path and storage `Close()` calls on every e2e run — [#4964](https://github.com/jaegertracing/jaeger/pull/4964) already flagged those as neglected.
- **No new architecture.** `coverage-<flag>` artifact → `gocovmerge` → `filter_coverage.py` → `Coverage Gate` is unchanged, and so is the Codecov upload.

The honest counterweight is that **the size of the gain is unknown**. The `direct` legs already exercise storage code in-process, and much of the extension wiring is unit-tested; the genuinely new coverage could be a fraction of a point. Unlike the `-coverpkg` question — where the answer is now known to be ≈0 — this has never been measured, because §3.2 guaranteed that every attempt to measure it collected an empty directory. That is why M4 is a measurement on one leg with an explicit decision gate, not a matrix-wide rollout.

| Criteria | Leave e2e uninstrumented | Restore `-coverpkg=./...` | `-cover` + `GOCOVERDIR` |
| --- | --- | --- | --- |
| Measures the e2e path | 🔴 nothing | 🔴 zeros and harness only | 🟢 |
| CI time | 🟢 | 🔴 restores the double compile | 🟡 net-neutral or better |
| Implementation risk | 🟢 | 🟢 | 🟡 shutdown rework, flush failures |
| Payoff known in advance | — | 🟢 known ≈0.03pp | 🔴 must be measured |

**Recommendation: `-cover` + `GOCOVERDIR`, gated on M4's measurement.** If M4 shows the new coverage is immaterial, M5 is dropped and the graceful-shutdown work from M3 is kept on its own merits.

---

## 6. Implementation plan

### M1 — Unstick pull requests today ⬜

**Set `after_n_builds: 18` → `12`** in [`.codecov.yml`](../../.codecov.yml). One line. Codecov resumes notifying, both `codecov/patch` and `codecov/project` reappear, every open pull request unblocks, and diff-level enforcement returns with them. No branch-protection change and no gate is given up.

The one wrinkle is ordering: `strict_yaml_branch: main` means Codecov honors only the copy of the config on `main`, so the change has no effect on its own pull request — that single pull request needs an administrator merge past the two stuck checks. One override, once, in exchange for restoring the gate rather than removing it.

The value is derived, not guessed: 12 is the number of uploads that carry non-ignored coverage (§2.1), confirmed against Codecov's reported session count on every commit since [#9086](https://github.com/jaegertracing/jaeger/pull/9086).

12 is an interim value that encodes a defect, not a target. The intended state is that all 27 uploads carry coverage and the threshold reads 27; M5 is what gets there, and M2 is what stops the discrepancy from being silent in the meantime. This milestone deliberately does not restore `-coverpkg=./...` to inflate the count back to 27, because that restores the *number* without restoring the *measurement* (§5).

### M2 — Enforce the upload invariant ⬜

The invariant this milestone establishes and defends is:

> `after_n_builds` equals the number of jobs calling `upload-codecov`, and **every one of those uploads carries coverage**.

A signal-free upload is not a quantity to subtract from the threshold — it is a defect in its own right. It makes "12 uploads arrived" stop meaning "the report is complete," which is the only property `after_n_builds` exists to guarantee (§1.3), and it silently degrades a job that appears to be contributing coverage. Any implementation that merely reconciles the number against however many uploads happen to carry data preserves the outage's root cause while hiding its symptom.

So the check is on the invariant, not on the arithmetic: add a step to the `summary-report` job in [`ci-summary-report.yml`](../../.github/workflows/ci-summary-report.yml) that, after [`filter_coverage.py`](../../scripts/e2e/filter_coverage.py) runs, fails if any downloaded `coverage-*` profile contains no non-ignored lines — naming the offending flags — and separately fails if the count of profiles disagrees with `after_n_builds` parsed from [`.codecov.yml`](../../.codecov.yml).

The fan-in is the right place because it is the only job that sees every profile and already applies the same exclusion list Codecov does. The distinction matters: #9086 left the number of `upload-codecov` invocations at 27 and changed only whether 15 of them carried anything, so a check counting invocations — the obvious implementation — would have passed throughout the outage.

Note the consequence for sequencing. On landing, this check **fails** on `main`, because the 15 e2e legs violate it today. That is the correct behavior and the reason M2 precedes M3–M5: it converts a silent stall into a standing, named defect that the fidelity work then closes. Whether it lands as blocking or as a warning that becomes blocking at M5 is the one open call in this milestone.

### M3 — Graceful shutdown of the spawned binary ⬜

Replace `Process.Kill()` in `Binary.Stop` ([`binary.go`](../../cmd/jaeger/internal/integration/binary.go)) with `SIGTERM` and a bounded wait, falling back to `SIGKILL` on timeout so a wedged process cannot hang CI. Prerequisite for M4 per §3.2, and independently exercises collector shutdown and storage `Close()`.

### M4 — Measure binary coverage on one leg ⬜

Instrument a single cheap e2e leg with no external stack — `memory-v2` or `badger e2e` — by building the spawned binary with `go build -cover`, pointing it at a `GOCOVERDIR`, and converting with `go tool covdata textfmt` into the existing `cover.out` path. Report how many lines and which packages are newly covered relative to that leg's current profile, and the change in compile time. **Decision gate:** proceed to M5 only if the additional coverage is material; otherwise close out the workstream, retaining M3.

### M5 — Roll out across the e2e matrix ⬜

Apply the M4 mechanism to the remaining e2e legs and drop the now-redundant second compile identified in [#9084](https://github.com/jaegertracing/jaeger/issues/9084) finding 2. Two consequences to manage: the contributing-upload count rises back toward 27, so `after_n_builds` must be realigned in the same pull request — which M2 now enforces rather than leaving to memory — and `Coverage Gate`'s cached `main` baseline steps up once, which the gate tolerates upward but which makes a later revert of this milestone fail the regression check.

### M6 — Diff-level gating in `Coverage Gate` ⬜

Add per-diff coverage to the `summary-report` job, closing the gap in §3.3 so the 95% patch target survives a Codecov outage instead of disappearing with it. The job already holds the merged, exclusion-filtered profile and can resolve the pull request base; the work is to intersect changed lines with the profile and apply the threshold, reported through the existing `Coverage Gate` check-run and sticky comment. `.codecov.yml`'s `ignore` list stays the single source of truth for exclusions, per ADR-004 Requirement 3.

Sequenced last because it is no longer urgent once M1 restores `codecov/patch`, and because it is the only milestone that would make retiring Codecov as a gate a real option rather than a downgrade (§4, option C). Whether to then stop *requiring* Codecov is left to M7; it should not be decided before this exists.

### M7 — Record the outcome ⬜

Once M1–M6 settle, capture the resulting arrangement — where coverage is measured, how it is merged, and what gates a pull request — in a new ADR superseding [ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md), and mark this RFC Implemented with a pointer to it.

---

## 7. Open questions

1. **Should the fan-in upload one merged profile instead of 27 per-job ones?** This is the alternative to M2 that eliminates the count rather than checking it: a single upload makes `after_n_builds: 1` permanently correct and the drift class impossible. The cost is Codecov's per-flag breakdown, which is what tells you *which* backend's tests cover a given line — genuinely useful, and not reproducible from a merged profile. M2 is proposed instead because it keeps that breakdown, but the trade is close enough to be worth arguing.
2. **Does `after_n_builds` remain the right mechanism at all?** It exists to prevent notification off a partial report ([#5811](https://github.com/jaegertracing/jaeger/pull/5811)). It is a count where what is actually wanted is "all uploads for this commit have arrived," which Codecov offers no direct expression of. If a better signal exists in Codecov's current API, it supersedes both M1 and M2.
3. **Does Codecov stay a *required* check after M6?** Only worth asking once a native diff gate exists. Keeping both is defense in depth against one system being down; dropping Codecov's contexts removes a third-party dependency from the merge path. Not decided here.
4. **Is per-leg binary instrumentation worth the flush risk?** A binary that fails to write its counter directory yields an empty profile, which the fan-in currently treats as a smaller merge rather than an error. M2's contributing-profile count would catch this, which is a second reason to land it before M5.

---

## 8. References

- [#9084](https://github.com/jaegertracing/jaeger/issues/9084) — Redundant builds in ES/OpenSearch e2e CI jobs; findings 2 and 3 are the basis of §1.2 and §5
- [#9086](https://github.com/jaegertracing/jaeger/pull/9086) — Skip unused ES image builds; removed `-coverpkg=./...` from the e2e target
- [#9088](https://github.com/jaegertracing/jaeger/pull/9088) — Skip es-index-cleaner/es-rollover image build for e2e cells
- [#9091](https://github.com/jaegertracing/jaeger/pull/9091) — Representative stuck pull request
- [#1516](https://github.com/jaegertracing/jaeger/issues/1516) — Use integration tests to generate coverage
- [#4964](https://github.com/jaegertracing/jaeger/pull/4964) — Introduced integration-test coverage and `after_n_builds`
- [#5811](https://github.com/jaegertracing/jaeger/pull/5811) — Documented the false positives caused by an under-set `after_n_builds`
- [ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md) — Migrating coverage gating to GitHub Actions
- [`.codecov.yml`](../../.codecov.yml) · [`ci-summary-report.yml`](../../.github/workflows/ci-summary-report.yml) · [`upload-codecov/action.yml`](../../.github/actions/upload-codecov/action.yml) · [`IntegrationTests.mk`](../../scripts/makefiles/IntegrationTests.mk) · [`binary.go`](../../cmd/jaeger/internal/integration/binary.go) · [`filter_coverage.py`](../../scripts/e2e/filter_coverage.py)
- [Go coverage for programs](https://go.dev/doc/build-cover) — `go build -cover`, `GOCOVERDIR`, `go tool covdata`

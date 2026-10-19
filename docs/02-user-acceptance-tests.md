# User Acceptance Tests

- [User Acceptance Tests](#user-acceptance-tests)
  - [How to Use This](#how-to-use-this)
  - [1 - Core + Headless](#1---core--headless)
  - [2 - TUI](#2---tui)
  - [3 - Web UI](#3---web-ui)
  - [4 - Extras](#4---extras)
  - [Cross-Cutting](#cross-cutting)
  - [The Nasty Cases](#the-nasty-cases)
  - [Sign-Off](#sign-off)

## How to Use This
- the **definition of done** for [01-specification.md](01-specification.md), one section per delivery phase
- each criterion is written so it can be **demonstrated**; tick `⬜` -> `✅` with the date, annotate skips in place
- `[A]` marks criteria with an automated test (fixture repos built in a temp dir), named in the criterion once it exists
- a criterion that needed a workaround to pass gets a note - workarounds are findings, not passes

## 1 - Core + Headless

| ✓   | #    | Criterion                                                                                                                                                                                                                                                                                       |
| --- | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ⬜   | 1.1  | With no config, `mrm status` in a folder of clones lists every repo under it, including repos nested in group folders (`platform/*`) with their group, and does not list folders that aren't repos (`logs/`) or anything inside a repo (submodules, `.worktrees/`) `[A] discover.TestWorkspace` |
| ⬜   | 1.2  | `mrm config init` writes a commented `~/.config/mrm/config.yaml`; `mrm config path` prints the config, state and cache paths, honouring `XDG_*` when set `[A] config.TestInitContentIsValid, TestWriteInitRefusesOverwrite`                                                                     |
| ⬜   | 1.3  | On a real tree of clones, every repo appears in `mrm status`, including ones a hand-kept list would have missed (cloned later, nested in a group folder); `mrm config init` offers a workspace per common clone folder that exists `[A] config.TestInitFindsCommonRoots`                        |
| ⬜   | 1.4  | The default branch comes from `origin/HEAD`; a `branch:` override in config wins `[A] engine.TestBranchOverride`                                                                                                                                                                                |
| ⬜   | 1.5  | Each primary state and flag in the spec is shown correctly on a fixture repo built for it `[A] git.TestStatePrecedence, TestParsePorcelainV2, engine.TestSync*`                                                                                                                                 |
| ⬜   | 1.6  | `mrm sync` on a clean repo behind its upstream fast-forwards it; no merge commit `[A] engine.TestSyncFastForwardsCleanDefault`                                                                                                                                                                  |
| ⬜   | 1.7  | `mrm sync` on a repo on a feature branch leaves the branch and working tree untouched and moves the local default branch to `origin/<default>` `[A] engine.TestSyncUpdatesDefaultInPlaceFromFeatureBranch`                                                                                      |
| ⬜   | 1.8  | `mrm sync` prints one live line per repo while running and ends with a summary: synced, current, skipped by reason, failed                                                                                                                                                                      |
| ⬜   | 1.9  | `mrm sync --dry-run` prints what each repo would do and changes no local branch and no working tree (it does fetch, so remote-tracking refs move) `[A] engine.TestSyncDryRun`                                                                                                                   |
| ⬜   | 1.10 | Exit code is `0` when all repos are synced or current, `2` when any needs attention, `1` on a bad config `[A] cmd.TestSyncExitCodes`                                                                                                                                                            |
| ⬜   | 1.11 | `-w`, `-t` and `--filter` narrow status and sync to the same set; `--filter "is:dirty -tag:archived"` works as written `[A] filter.TestMatch, cmd.TestStatusJSONAndFilter`                                                                                                                      |
| ⬜   | 1.12 | `mrm status --json` is valid JSON with every field in [Status](01-specification.md#status) `[A] cmd.TestStatusJSONAndFilter`                                                                                                                                                                    |
| ⬜   | 1.13 | `mrm open <repo>`, `--pr` and `--ci` open the right page for a GitHub, a GitLab and a Bitbucket repo (on WSL in the Windows browser) `[A] forge.TestURLs, cmd.TestOpenPrint` for the URLs                                                                                                       |
| ⬜   | 1.14 | With `notify.on: attention`, a run with a skipped repo sends one Gotify message naming it; a clean run sends nothing; `always` sends both `[A] notify.TestSend` for delivery                                                                                                                    |
| ⬜   | 1.15 | Each run is appended to `runs.jsonl` in the state dir with start, duration and outcome per repo                                                                                                                                                                                                 |
| ⬜   | 1.16 | `mrm doctor` reports git version, config validity, missing roots, repos with no `origin/HEAD`, ssh agent with no keys and a world-writable config                                                                                                                                               |
| ⬜   | 1.17 | `mrm sync` replaces a hand-rolled update script in daily use for at least a week, on more than one workspace                                                                                                                                                                                    |
| ⬜   | 1.18 | A fetch refreshes a stale `origin/HEAD` (seen on real repos: `develop` locally, `master` on the forge), at most once a day per repo; a sync of ~80 repos with nothing to pull takes under 30 s `[A] engine.TestFetchRefreshesStaleDefault`                                                      |
| ⬜   | 1.19 | Tab completes workspaces after `-w`, tags after `-t`, filter terms after `-f` (keys, `is:` values, your workspaces, tags and groups) and repo names for `mrm open` `[A] cmd.TestCompletion`                                                                                                     |
| ⬜   | 1.20 | `forge:` matches self-hosted hosts mapped under `forges:` `[A] filter.TestForgeUsesSelfHostedMapping`                                                                                                                                                                                           |

## 2 - TUI

| ✓   | #    | Criterion                                                                                                                                                     |
| --- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ⬜   | 2.1  | `mrm` on a terminal opens the TUI from the cache in under 150 ms, then refreshes rows in place (measured 13 ms for 86 repos; `mrm -v` logs "tui first frame") |
| ⬜   | 2.2  | Rows are grouped (workspace, then group); `tab` collapses and expands a group `[A] tui.TestRender, TestCollapse`                                              |
| ⬜   | 2.3  | `/` filters as you type with the same language as `--filter`; `esc` clears `[A] tui.TestFilter`                                                               |
| ⬜   | 2.4  | `ctrl+k` opens the palette; every action and workspace is reachable from it by fuzzy search `[A] tui.TestPaletteAndWorkspaces, TestFuzzyRank`                 |
| ⬜   | 2.5  | `space` marks, `a` marks all visible, `s` syncs the marked repos with a spinner per row and live result glyphs `[A] tui.TestMarks, TestSyncFromTUI`           |
| ⬜   | 2.6  | A sync with skipped or failed repos ends on the needs-attention view, grouped by reason; `!` opens it any time `[A] tui.TestAttentionView`                    |
| ⬜   | 2.7  | `enter` opens lazygit in the repo; quitting lazygit returns to mrm with that row refreshed                                                                    |
| ⬜   | 2.8  | `e`, `t`, `o`, `O`, `l`, `w`, `?` and `q` do what the key table says                                                                                          |
| ⬜   | 2.9  | Usable at 80 x 24: no wrapped rows, columns truncate with `…` `[A] tui.TestRender`                                                                            |
| ⬜   | 2.10 | `NO_COLOR=1` gives a readable glyph-only view; light and dark terminals are both readable                                                                     |

## 3 - Web UI

| ✓   | #   | Criterion                                                                                                                                                                                                                                           |
| --- | --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ⬜   | 3.1 | `mrm web --open` opens the Overview in the browser (Windows browser on WSL)                                                                                                                                                                         |
| ⬜   | 3.2 | The Overview shows state counts per workspace, the needs-attention list, work at risk (long groups capped with a "+ N more" filter link) and the filterable table `[A] web.TestOverview`                                                            |
| ⬜   | 3.3 | Starting a sync from the web updates rows live over SSE in every open tab; a sync started from the TUI or CLI shows in the top bar while it runs and its results appear when it finishes `[A] web.TestSyncStreamsAndRecords, TestExternalLockShown` |
| ⬜   | 3.4 | The Repo page shows branches, stashes, last 20 commits, last sync results and working forge links `[A] web.TestRepoPage`                                                                                                                            |
| ⬜   | 3.5 | The Activity page lists past runs from `runs.jsonl` with outcome per repo `[A] web.TestSyncStreamsAndRecords`                                                                                                                                       |
| ⬜   | 3.6 | The Settings page shows the loaded config with secrets hidden, and text size and theme apply at once and stick per browser `[A] web.TestSettingsHidesSecrets` for the config                                                                        |
| ⬜   | 3.7 | Light, dark and system themes; every page at 360, 768 and 1440 px with no horizontal page scroll (checked 2026-10-15 in Chrome at 1440, 768 and 518 px - Chrome's minimum window; below that the same layout applies)                               |

## 4 - Extras

| ✓   | #   | Criterion                                                                                                                                                                                                                                                                                                                              |
| --- | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ⬜   | 4.1 | `mrm exec -t terraform -- terraform fmt -check` runs in every terraform repo in parallel, output prefixed by repo, exit code non-zero if any failed; asks first (refuses without a terminal unless `--yes`) `[A] fleet.TestExecParallelPrefixes, TestExecFailures, TestExecRunsInRepo, TestExecCancel, cmd.TestConfirmWithoutTerminal` |
| ⬜   | 4.2 | A `post_sync` hook runs only in repos matching `if_file` and only when `HEAD` moved; a failing hook is reported without failing the sync `[A] engine.TestPostSyncHooks`                                                                                                                                                                |
| ⬜   | 4.3 | `mrm prune` lists merged and upstream-gone branches and deletes only what is confirmed; never the current or default branch; without a terminal it refuses unless `--yes` `[A] engine.TestPrune, cmd.TestConfirmWithoutTerminal` (dry run checked 2026-10-19 on a real tree: 42 merged branches in 22 repos)                           |
| ⬜   | 4.4 | `mrm drift` lists exact `repos:` keys missing on disk and, with a forge token, org repos not cloned, each with a clone command; archived ones only with `--archived` `[A] drift.TestStaleKeys, TestCompareOrg, forgeapi.TestOrgRepos` (checked 2026-10-19 against a real GitHub account: 93 listed, 45 not cloned, 18 archived)        |
| ⬜   | 4.5 | `mrm schedule install -w work --on "Mon..Fri 08:30"` creates and enables a user timer; it runs while no terminal is open and catches up after the machine was off; `schedule remove` removes it `[A] schedule.TestInstallListRemove, TestBadCalendar` for the units                                                                    |
| ⬜   | 4.6 | With forge tokens configured, open PR count and last CI status show in status, the TUI and the web UI; hosts without a token are never contacted `[A] forgeapi.TestInfoPerForge, TestCache, TestTokens, tui.TestForgeInfo, web.TestForgeInfo` (checked 2026-10-19 on 37 real GitHub repos: 2.4 s)                                      |

## Cross-Cutting

| ✓   | #   | Criterion                                                                                                                                                                                                                  |
| --- | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ⬜   | X.1 | No secret appears in config, logs, `--verbose` output or the web UI; config only accepts `*_cmd` / `*_env` for secrets `[A] config.TestLiteralTokenRefusedWithHint, notify.TestTokenErrorsDontLeakOutput, notify.TestSend` |
| ⬜   | X.2 | The web UI refuses requests without the token, with a `Host` other than `127.0.0.1`/`localhost`, and POSTs without the token header `[A] web.TestSecurity`                                                                 |
| ⬜   | X.3 | The web UI is not reachable from another machine on the LAN                                                                                                                                                                |
| ⬜   | X.4 | `task check` (fmt, vet, race tests) is clean; release binaries for linux/darwin amd64/arm64 build                                                                                                                          |
| ⬜   | X.5 | `go install github.com/adegoodyer/mrm@latest` works on a clean machine                                                                                                                                                     |

## The Nasty Cases

| ✓   | #    | Case                                                                                                       | Expected                                                                                                                                                                                   |
| --- | ---- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| ⬜   | N.1  | Dirty tree on the default branch, remote ahead                                                             | skipped `dirty`; working tree and index identical after `[A] engine.TestSyncSkipsDirty`                                                                                                    |
| ⬜   | N.2  | Local default branch has commits not on the remote, remote also moved                                      | skipped `diverged`; no merge commit, no reset `[A] engine.TestSyncSkipsDiverged`                                                                                                           |
| ⬜   | N.3  | Rebase or merge in progress                                                                                | fetch only, state `conflict` `[A] engine.TestSyncSkipsMergeInProgress`                                                                                                                     |
| ⬜   | N.4  | Detached `HEAD`                                                                                            | fetch only, flag `detached` `[A] engine.TestSyncSkipsDetached`                                                                                                                             |
| ⬜   | N.5  | No `origin/HEAD`                                                                                           | falls back to `main` then `master`; `doctor` lists the repo                                                                                                                                |
| ⬜   | N.6  | ssh key not loaded / Bitbucket rejects the key                                                             | each affected repo fails within the timeout with a plain reason (e.g. "remote repo not found, or no access"); no prompt, no hang `[A] engine.TestFetchFailsFast, git.TestErrorMessage`     |
| ⬜   | N.7  | Network offline                                                                                            | every fetch fails fast; the summary says "offline?" once instead of 80 identical errors                                                                                                    |
| ⬜   | N.8  | Upstream branch deleted on the remote                                                                      | flag `upstream-gone`; fast-forward skipped; listed by `prune` `[A] engine.TestUpstreamGone`                                                                                                |
| ⬜   | N.9  | Repo with no remote                                                                                        | status works, flag `no-remote`, sync skips it quietly `[A] engine.TestSyncNoRemote`                                                                                                        |
| ⬜   | N.10 | A scheduled sync and a manual sync of one workspace at once                                                | the second exits at once with "sync already running for workspace … (pid …)" `[A] engine.TestLock`                                                                                         |
| ⬜   | N.11 | One huge repo with a slow fetch                                                                            | it times out alone; the other repos finish on time `[A] engine.TestTimeout`                                                                                                                |
| ⬜   | N.12 | A repo deleted or moved while the TUI or web UI is open                                                    | its row shows `error: missing` and drops on the next refresh                                                                                                                               |
| ⬜   | N.13 | Repo path with spaces or non-ASCII characters                                                              | listed, synced and opened correctly `[A] engine.TestSyncOddPath`                                                                                                                           |
| ⬜   | N.14 | Untracked files on a clean default branch that is behind                                                   | fast-forwarded; untracked files untouched (unless the incoming commit adds the same path, then failed with git's message and the file kept) `[A] engine.TestSyncUntracked`                 |
| ⬜   | N.15 | `switch_merged: true` and a brand-new branch with no commits yet (an ancestor of the default, so "merged") | left alone: only a branch that was pushed is switched away from `[A] engine.TestSwitchMerged`                                                                                              |
| ⬜   | N.16 | A local branch whose upstream was deleted on the forge and pruned by a fetch (an old `develop`)            | not counted as unpushed; flag `stale-branches`, "1 stale branch" in the row; a never-pushed branch still counts `[A] engine.TestStaleBranchIsNotUnpushed, TestNeverPushedBranchIsUnpushed` |

## Sign-Off
mrm is done for a phase when every box in that phase and the cross-cutting rows it touches are ticked or annotated.

| Area                | Criteria   | Ticked | Walked through |
| ------------------- | ---------- | ------ | -------------- |
| 1 - core + headless | 1.1 - 1.20 | 0 / 20 | ⬜              |
| 2 - TUI             | 2.1 - 2.10 | 0 / 10 | ⬜              |
| 3 - web UI          | 3.1 - 3.7  | 0 / 7  | ⬜              |
| 4 - extras          | 4.1 - 4.6  | 0 / 6  | ⬜              |
| cross-cutting       | X.1 - X.5  | 0 / 5  | ⬜              |
| nasty cases         | N.1 - N.16 | 0 / 16 | ⬜              |
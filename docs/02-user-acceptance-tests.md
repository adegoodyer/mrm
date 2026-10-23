# User Acceptance Tests

- [User Acceptance Tests](#user-acceptance-tests)
  - [How to Use This](#how-to-use-this)
  - [1 - Core + Headless](#1---core--headless)
  - [2 - TUI](#2---tui)
  - [3 - Web UI](#3---web-ui)
  - [4 - Extras](#4---extras)
  - [5 - Polish](#5---polish)
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
| ✅ 2026-10-23 | 1.1  | With no config, `mrm status` in a folder of clones lists every repo under it, including repos nested in group folders (`platform/*`) with their group, and does not list folders that aren't repos (`logs/`) or anything inside a repo (submodules, `.worktrees/`) `[A] discover.TestWorkspace` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.2  | `mrm config init` writes a commented `~/.config/mrm/config.yaml`; `mrm config path` prints the config, state and cache paths, honouring `XDG_*` when set `[A] config.TestInitContentIsValid, TestWriteInitRefusesOverwrite` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.3  | On a real tree of clones, every repo appears in `mrm status`, including ones a hand-kept list would have missed (cloned later, nested in a group folder); `mrm config init` offers a workspace per common clone folder that exists `[A] config.TestInitFindsCommonRoots` (checked 2026-10-23 on a real tree: 138 repos; the only `.git` folders not listed are nested inside other repos) |
| ✅ 2026-10-23 | 1.4  | The default branch comes from `origin/HEAD`; a `branch:` override in config wins `[A] engine.TestBranchOverride` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.5  | Each primary state and flag in the spec is shown correctly on a fixture repo built for it `[A] git.TestStatePrecedence, TestParsePorcelainV2, engine.TestSync*` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.6  | `mrm sync` on a clean repo behind its upstream fast-forwards it; no merge commit `[A] engine.TestSyncFastForwardsCleanDefault` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.7  | `mrm sync` on a repo on a feature branch leaves the branch and working tree untouched and moves the local default branch to `origin/<default>` `[A] engine.TestSyncUpdatesDefaultInPlaceFromFeatureBranch` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.8  | `mrm sync` prints one live line per repo while running and ends with a summary: synced, current, skipped by reason, failed |
| ✅ 2026-10-23 | 1.9  | `mrm sync --dry-run` prints what each repo would do and changes no local branch and no working tree (it does fetch, so remote-tracking refs move) `[A] engine.TestSyncDryRun` (checked 2026-10-23: the dry run left a repo 1 behind at the same commit) |
| ✅ 2026-10-23 | 1.10 | Exit code is `0` when all repos are synced or current, `2` when any needs attention, `1` on a bad config `[A] cmd.TestSyncExitCodes` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.11 | `-w`, `-t` and `--filter` narrow status and sync to the same set; `--filter "is:dirty -tag:archived"` works as written `[A] filter.TestMatch, cmd.TestStatusJSONAndFilter` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.12 | `mrm status --json` is valid JSON with every field in [Status](01-specification.md#status) `[A] cmd.TestStatusJSONAndFilter` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 1.13 | `mrm open <repo>`, `--pr` and `--ci` open the right page for a GitHub, a GitLab and a Bitbucket repo (on WSL in the Windows browser) `[A] forge.TestURLs, cmd.TestOpenPrint` for the URLs (checked 2026-10-23 in the Windows browser for GitHub and GitLab; Bitbucket by the URL test) |
| ⬜   | 1.14 | With `notify.on: attention`, a run with a skipped repo sends one Gotify message naming it; a clean run sends nothing; `always` sends both `[A] notify.TestSend` for delivery                                                                                                                    |
| ✅ 2026-10-23 | 1.15 | Each run is appended to `runs.jsonl` in the state dir with start, duration and outcome per repo |
| ✅ 2026-10-23 | 1.16 | `mrm doctor` reports git version, config validity, missing roots, repos with no `origin/HEAD`, ssh agent with no keys and a world-writable config |
| ✅ 2026-10-23 | 1.17 | `mrm sync` replaces a hand-rolled update script in daily use for at least a week, on more than one workspace (signed off by the owner 2026-10-23) |
| ✅ 2026-10-23 | 1.18 | A fetch refreshes a stale `origin/HEAD` (seen on real repos: `develop` locally, `master` on the VCS host), at most once a day per repo; a sync of ~80 repos with nothing to pull takes under 30 s `[A] engine.TestFetchRefreshesStaleDefault` (checked 2026-10-23: 110 repos in 19.9 s, after the 30 s timeout, ssh keepalives and slowest-first; a slow gitlab.com had pushed it to 41 s before) |
| ✅ 2026-10-23 | 1.19 | Tab completes workspaces after `-w`, tags after `-t`, filter terms after `-f` (keys, `is:` values, your workspaces, tags and groups) and repo names for `mrm open` `[A] cmd.TestCompletion` |
| ✅ 2026-10-23 | 1.20 | `host:` (and its old name `forge:`) matches self-hosted hosts mapped under `hosts:` (or the old `forges:`) `[A] filter.TestForgeUsesSelfHostedMapping, TestHostTerm, config.TestOldHostNames` (automated tests pass, 2026-10-23) |

## 2 - TUI

| ✓   | #    | Criterion                                                                                                                                                     |
| --- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-23 | 2.1  | `mrm` on a terminal opens the TUI from the cache in under 150 ms, then refreshes rows in place (measured 13 ms for 86 repos; `mrm -v` logs "tui first frame") (checked 2026-10-23: 7-8 ms for 140 repos) |
| ✅ 2026-10-23 | 2.2  | Rows are grouped (workspace, then group); `tab` collapses and expands a group `[A] tui.TestRender, TestCollapse` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.3  | `/` filters as you type with the same language as `--filter`; `esc` clears `[A] tui.TestFilter` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.4  | `ctrl+k` opens the palette; every action and workspace is reachable from it by fuzzy search `[A] tui.TestPaletteAndWorkspaces, TestFuzzyRank` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.5  | `space` marks, `a` marks all visible, `s` syncs the marked repos with a spinner per row and live result glyphs `[A] tui.TestMarks, TestSyncFromTUI` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.6  | A sync with skipped or failed repos ends on the needs-attention view, grouped by reason; `!` opens it any time `[A] tui.TestAttentionView` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.7  | `enter` opens lazygit in the repo; quitting lazygit returns to mrm with that row refreshed |
| ✅ 2026-10-23 | 2.8  | `e`, `t`, `o`, `p`, `P`, `b`, `l`, `w`, `?` and `q` do what the key table says |
| ✅ 2026-10-23 | 2.9  | Usable at 80 x 24: no wrapped rows, columns truncate with `…` `[A] tui.TestRender` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 2.10 | `NO_COLOR=1` gives a readable glyph-only view; light and dark terminals are both readable |

## 3 - Web UI

| ✓   | #   | Criterion                                                                                                                                                                                                                                           |
| --- | --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-23 | 3.1 | `mrm web --open` opens the Overview in the browser (Windows browser on WSL) |
| ✅ 2026-10-23 | 3.2 | The Overview shows state counts per workspace, the needs-attention list, work at risk (long groups capped with a "+ N more" filter link) and the filterable table `[A] web.TestOverview` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 3.3 | Starting a sync from the web updates rows live over SSE in every open tab; a sync started from the TUI or CLI shows in the top bar while it runs and its results appear when it finishes `[A] web.TestSyncStreamsAndRecords, TestExternalLockShown` |
| ⬜   | 3.4 | The Repo page shows branches, stashes, last 20 commits, last sync results and working links to the repo, its PRs and its CI `[A] web.TestRepoPage`                                                                                                  |
| ✅ 2026-10-23 | 3.5 | The Activity page lists past runs from `runs.jsonl` with outcome per repo `[A] web.TestSyncStreamsAndRecords` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 3.6 | The Settings page shows the loaded config with secrets hidden, and text size and theme apply at once and stick per browser `[A] web.TestSettingsHidesSecrets` for the config |
| ✅ 2026-10-23 | 3.7 | Light, dark and system themes; every page at 360, 768 and 1440 px with no horizontal page scroll (checked 2026-10-15 in Chrome at 1440, 768 and 518 px - Chrome's minimum window; below that the same layout applies) (and 2026-10-23 at 360 px in Chrome device mode, light and dark) |

## 4 - Extras

| ✓   | #   | Criterion                                                                                                                                                                                                                                                                                                                              |
| --- | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-23 | 4.1 | `mrm exec -t terraform -- terraform fmt -check` runs in every terraform repo in parallel, output prefixed by repo, exit code non-zero if any failed; asks first (refuses without a terminal unless `--yes`) `[A] fleet.TestExecParallelPrefixes, TestExecFailures, TestExecRunsInRepo, TestExecCancel, cmd.TestConfirmWithoutTerminal` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 4.2 | A `post_sync` hook runs only in repos matching `if_file` and only when `HEAD` moved; a failing hook is reported without failing the sync `[A] engine.TestPostSyncHooks` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 4.3 | `mrm prune` lists merged and upstream-gone branches and deletes only what is confirmed; never the current or default branch; without a terminal it refuses unless `--yes` `[A] engine.TestPrune, cmd.TestConfirmWithoutTerminal` (dry run checked 2026-10-19 on a real tree: 42 merged branches in 22 repos) (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 4.4 | `mrm drift` lists exact `repos:` keys missing on disk and, with an API token, org repos not cloned, each with a clone command; archived ones only with `--archived` `[A] drift.TestStaleKeys, TestCompareOrg, forgeapi.TestOrgRepos` (checked 2026-10-19 against a real GitHub account: 93 listed, 45 not cloned, 18 archived) (automated tests pass, 2026-10-23) |
| ⬜   | 4.5 | `mrm schedule install -w work --on "Mon..Fri 08:30"` creates and enables a user timer; it runs while no terminal is open and catches up after the machine was off; `schedule remove` removes it `[A] schedule.TestInstallListRemove, TestBadCalendar` for the units                                                                    |
| ⬜   | 4.6 | With API tokens configured, open PR count and last CI status show in status, the TUI and the web UI; hosts without a token are never contacted `[A] forgeapi.TestInfoPerForge, TestCache, TestTokens, tui.TestForgeInfo, web.TestForgeInfo` (checked 2026-10-19 on 37 real GitHub repos: 2.4 s)                                        |

## 5 - Polish

| ✓   | #    | Criterion                                                                                                                                                                                                                                       |
| --- | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-23 | 5.1  | Someone new to mrm can install it and sync from the README alone; the pictures match what mrm shows in light and dark (signed off by the owner 2026-10-23) |
| ✅ 2026-10-23 | 5.2  | `?` help, the command palette and the user guide list the same keys `[A] tui.TestKeysDocumented, TestKeysUnique` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.3  | `←` `→` step through the workspaces and wrap, with *all* among them; `shift+tab` collapses every group, then expands them; `J` `K` and `shift+↑` `shift+↓` move 10 `[A] tui.TestWorkspaceCycleAndGroups` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.4  | `o`, `p`, `P` and `b` open the repo, its pull requests, its pipelines and the current branch in the browser, on GitHub, GitLab and Bitbucket (checked 2026-10-23 on GitHub from the TUI; GitLab and Bitbucket share the URLs checked under 1.13) |
| ✅ 2026-10-23 | 5.5  | The footer shows more keys on a wider terminal, always `? help`, and different keys on a group header and with repos marked `[A] tui.TestFooter` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.6  | `L` shows the commit graph with every branch and remote; `esc` closes it; `ctrl+d` `ctrl+u` scroll it and the detail pane `[A] tui.TestGraph` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.7  | A click selects a row and the wheel scrolls; `tui.mouse: false` gives the mouse back to the terminal `[A] tui.TestMouse` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.8  | Quitting and reopening `mrm` brings back the workspace, collapsed groups and detail pane; `-w` still wins `[A] tui.TestViewRemembered` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.9  | With `ff_dirty: true`, a dirty repo behind its remote is fast-forwarded when the update touches none of the changed files, keeping the edits, and skipped naming the files when it does; `--dry-run` says the same `[A] engine.TestSyncFFDirty` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | 5.10 | `mrm sync --dry-run` says `would fast-forward main by 3 commits`, and a real sync `fast-forwarded main by 3 commits` `[A] engine.TestSyncDryRun` |
| ✅ 2026-10-23 | 5.11 | With repos marked, `o` `p` `P` `b` open a page for each; more than 5 asks first in mrm's own dialog (`y` / `n`) `[A] tui.TestOpenMarked` |
| ✅ 2026-10-23 | 5.12 | `d` asks once, listing the repos and flagging work that exists only here; `y` moves them to the trash (restorable), `n` leaves them; a workspace root is never moved `[A] tui.TestTrash, trash.TestMove` (checked 2026-10-23: the folder landed in the desktop trash with its original path) |
| ✅ 2026-10-23 | 5.13 | No screen, message or help text says "forge"; the web Repo page says "Open on GitHub" (or Bitbucket...); a config with the old `forge_tokens:` still works and `mrm doctor` suggests `host_tokens:` `[A] config.TestOldHostNames` (checked 2026-10-23: no user-facing string says forge; the Repo page says "Open on GitHub"; a config with `forge_tokens:` loads and `doctor` says to rename it) |
| ⬜   | 5.14 | In the web UI, `/` focuses the filter (from another page it goes to the Overview first), `esc` clears it and then leaves it, and `?` opens and closes a list of keys; none of them fire while typing in the filter |

## Cross-Cutting

| ✓   | #   | Criterion                                                                                                                                                                                                                  |
| --- | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-23 | X.1 | No secret appears in config, logs, `--verbose` output or the web UI; config only accepts `*_cmd` / `*_env` for secrets `[A] config.TestLiteralTokenRefusedWithHint, notify.TestTokenErrorsDontLeakOutput, notify.TestSend` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | X.2 | The web UI refuses requests without the token, with a `Host` other than `127.0.0.1`/`localhost`, and POSTs without the token header `[A] web.TestSecurity` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | X.3 | The web UI is not reachable from another machine on the LAN (checked 2026-10-23: listening on 127.0.0.1:7420 only; the LAN address refuses the connection) |
| ✅ 2026-10-23 | X.4 | `task check` (fmt, vet, race tests) is clean; release binaries for linux/darwin amd64/arm64 build (checked 2026-10-23: `task check` clean; `task release:snapshot` built all four archives) |
| ⬜   | X.5 | `go install github.com/adegoodyer/mrm@latest` works on a clean machine                                                                                                                                                     |

## The Nasty Cases

| ✓   | #    | Case                                                                                                       | Expected                                                                                                                                                                                   |
| --- | ---- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| ✅ 2026-10-23 | N.1  | Dirty tree on the default branch, remote ahead                                                             | skipped `dirty`; working tree and index identical after `[A] engine.TestSyncSkipsDirty` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.2  | Local default branch has commits not on the remote, remote also moved                                      | skipped `diverged`; no merge commit, no reset `[A] engine.TestSyncSkipsDiverged` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.3  | Rebase or merge in progress                                                                                | fetch only, state `conflict` `[A] engine.TestSyncSkipsMergeInProgress` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.4  | Detached `HEAD`                                                                                            | fetch only, flag `detached` `[A] engine.TestSyncSkipsDetached` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.5  | No `origin/HEAD`                                                                                           | falls back to `main` then `master`; `doctor` lists the repo (checked 2026-10-23: `doctor` listed 2 real repos with the fix) |
| ✅ 2026-10-23 | N.6  | ssh key not loaded / Bitbucket rejects the key                                                             | each affected repo fails within the timeout with a plain reason (e.g. "remote repo not found, or no access"); no prompt, no hang `[A] engine.TestFetchFailsFast, git.TestErrorMessage` (checked 2026-10-23: 6 real repos gone or without access, each "remote repo not found, or no access") |
| ✅ 2026-10-23 | N.7  | Network offline                                                                                            | every fetch fails fast; the summary says "offline?" once instead of 80 identical errors (checked 2026-10-23 with no network: 28 repos fail in 0.7 s; after a fix the same day, needs-attention lists each reason once with a count) `[A] cmd.TestSummaryOffline` |
| ✅ 2026-10-23 | N.8  | Upstream branch deleted on the remote                                                                      | flag `upstream-gone`; fast-forward skipped; listed by `prune` `[A] engine.TestUpstreamGone` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.9  | Repo with no remote                                                                                        | status works, flag `no-remote`, sync skips it quietly `[A] engine.TestSyncNoRemote` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.10 | A scheduled sync and a manual sync of one workspace at once                                                | the second exits at once with "sync already running for workspace … (pid …)" `[A] engine.TestLock` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.11 | One huge repo with a slow fetch                                                                            | it times out alone; the other repos finish on time `[A] engine.TestTimeout` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.12 | A repo deleted or moved while the TUI or web UI is open                                                    | its row shows `error: missing` and drops on the next refresh (failed first on 2026-10-23: the TUI and the web UI only looked for repos at start; fixed the same day - `r` or a page load now drops it at once) `[A] tui.TestRereadFindsRepos, web.TestRediscover` |
| ✅ 2026-10-23 | N.13 | Repo path with spaces or non-ASCII characters                                                              | listed, synced and opened correctly `[A] engine.TestSyncOddPath` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.14 | Untracked files on a clean default branch that is behind                                                   | fast-forwarded; untracked files untouched (unless the incoming commit adds the same path, then failed with git's message and the file kept) `[A] engine.TestSyncUntracked` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.15 | `switch_merged: true` and a brand-new branch with no commits yet (an ancestor of the default, so "merged") | left alone: only a branch that was pushed is switched away from `[A] engine.TestSwitchMerged` (automated tests pass, 2026-10-23) |
| ✅ 2026-10-23 | N.16 | A local branch whose upstream was deleted on the VCS host and pruned by a fetch (an old `develop`)         | not counted as unpushed; flag `stale-branches`, "1 stale branch" in the row; a never-pushed branch still counts `[A] engine.TestStaleBranchIsNotUnpushed, TestNeverPushedBranchIsUnpushed` (automated tests pass, 2026-10-23) |

## Sign-Off
mrm is done for a phase when every box in that phase and the cross-cutting rows it touches are ticked or annotated.

| Area                | Criteria   | Ticked  | Walked through |
| ------------------- | ---------- | ------- | -------------- |
| 1 - core + headless | 1.1 - 1.20 | 19 / 20 | ⬜              |
| 2 - TUI             | 2.1 - 2.10 | 10 / 10 | ✅              |
| 3 - web UI          | 3.1 - 3.7  | 6 / 7   | ⬜              |
| 4 - extras          | 4.1 - 4.6  | 4 / 6   | ⬜              |
| 5 - polish          | 5.1 - 5.14 | 13 / 14 | ⬜              |
| cross-cutting       | X.1 - X.5  | 4 / 5   | ⬜              |
| nasty cases         | N.1 - N.16 | 16 / 16 | ✅              |
# Roadmap

- [Roadmap](#roadmap)
  - [How to Use This](#how-to-use-this)
  - [Phase 0 - Setup](#phase-0---setup)
  - [Phase 1 - Core + Headless](#phase-1---core--headless)
  - [Phase 2 - TUI](#phase-2---tui)
  - [Phase 3 - Web UI](#phase-3---web-ui)
  - [Phase 4 - Extras](#phase-4---extras)
  - [Phase 5 - Polish](#phase-5---polish)
  - [Releases](#releases)
  - [Follow-Ups](#follow-ups)

## How to Use This
The [spec](01-specification.md) says *what*, this says *in what order*. Each phase ends usable; its definition of done is the matching section of [02-user-acceptance-tests.md](02-user-acceptance-tests.md). Work happens on `next-release`, a commit per change, each noted under `[Unreleased]` in [CHANGELOG.md](../CHANGELOG.md).

## Phase 0 - Setup

| ✓            | #   | Step                                                                                                                                                                       |
| ------------ | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-14 | 0.1 | spec, UAT and roadmap                                                                                                                                                      |
| ✅ 2026-10-14 | 0.2 | `adegoodyer/mrm` on GitHub (private until it is ready to publish)                                                                                                          |
| ✅ 2026-10-14 | 0.3 | skeleton from the xdg-viper-cobra-zap template (minus viper): `go.mod`, cobra root, `version`, `config path`, Taskfile (`check`, `build`, `test:unit`), README, README-dev |

## Phase 1 - Core + Headless

| ✓            | #    | Step                                                                                                                                                                                |
| ------------ | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-14 | 1.1  | test harness: fixture repos (bare remote + seed + clones) for every state and nasty case (`internal/gittest`, isolated from the developer's git config)                             |
| ✅ 2026-10-14 | 1.2  | `internal/config`: XDG paths, strict YAML load and validation, workspaces, repo overrides by name or glob, `config init/edit/path`                                                  |
| ✅ 2026-10-14 | 1.3  | `internal/discover`: roots, depth, ignore globs, groups, stop at a repo                                                                                                             |
| ✅ 2026-10-14 | 1.4  | `internal/git`: runner (no prompts, BatchMode, timeouts, process-group kill); `porcelain=v2` parser; unpushed commits; default branch resolution; plain reasons for remote failures |
| ✅ 2026-10-14 | 1.5  | `internal/engine`: worker pool, events, workspace lock; status, fetch (refreshing `origin/HEAD`) and sync per the spec's step table                                                 |
| ✅ 2026-10-14 | 1.6  | filter language, shared by every front end                                                                                                                                          |
| ✅ 2026-10-14 | 1.7  | `status`, `fetch`, `sync` (live lines, summary, `--dry-run`, exit codes), `--json`                                                                                                  |
| ✅ 2026-10-14 | 1.8  | cache and `runs.jsonl` history                                                                                                                                                      |
| ✅ 2026-10-14 | 1.9  | forge detection and `open` (`--pr`, `--ci`, `--branch`, `--print`; `wslview` on WSL)                                                                                                |
| ✅ 2026-10-14 | 1.10 | notifications: Gotify, ntfy; `token_cmd` / `token_env`                                                                                                                              |
| ✅ 2026-10-14 | 1.11 | `doctor`                                                                                                                                                                            |
| ⬜            | 1.12 | dogfood: own config with a work and a personal workspace; a week of daily use instead of a hand-rolled update script                                                                |
| ⬜            | 1.13 | UAT walkthrough of phase 1                                                                                                                                                          |

First run on a real tree (2026-10-14, read-only `sync --dry-run`): 86 repos across Bitbucket and GitLab in 31.9 s - 19 would fast-forward, 64 current, 1 skipped (dirty), 2 failed (no access, or the repo is gone). `status`: 0.46 s warm.

## Phase 2 - TUI

| ✓            | #   | Step                                                                                                                                                                                    |
| ------------ | --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-15 | 2.1 | TUI library: tview on tcell, never Bubble Tea (Q2 closed); the list is a custom primitive                                                                                               |
| ✅ 2026-10-15 | 2.2 | list model: grouped rows, glyphs, column layout and truncation, cache-first paint (13 ms for 86 repos), live updates from engine events; shared row text with the CLI (`internal/view`) |
| ✅ 2026-10-15 | 2.3 | filter, marks, sync/fetch actions (workspace lock, cache, history), needs-attention view                                                                                                |
| ✅ 2026-10-15 | 2.4 | command palette (ranked fuzzy search), detail pane (branches, stashes, commits), workspace switcher, help                                                                               |
| ✅ 2026-10-15 | 2.5 | hand-off to lazygit, `$EDITOR`, shell (suspend the screen, run, resume, refresh the row)                                                                                                |
| ✅ 2026-10-15 | 2.6 | themes (terminal palette), `NO_COLOR` / `--no-color`, 80 x 24; tests on a simulated screen, race-clean                                                                                  |
| ✅ 2026-10-15 | 2.7 | unpushed counts only work that exists only here; stale branches (upstream gone) get their own flag - found on the first real TUI run                                                    |
| ⬜            | 2.8 | UAT walkthrough of phase 2 in a real terminal (light and dark themes, lazygit hand-off)                                                                                                 |

## Phase 3 - Web UI

| ✓            | #   | Step                                                                                                                                                                                 |
| ------------ | --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| ✅ 2026-10-15 | 3.1 | HTML mockups of Overview, Repo, Activity and Settings (desktop and phone, light and dark); text enlarged and a text-size setting added on review; approved                           |
| ✅ 2026-10-15 | 3.2 | server: 127.0.0.1 bind, token to HttpOnly cookie, Host check, token header on POST, strict CSP; embedded templates, plain JS (not htmx), CSS and fonts                               |
| ✅ 2026-10-15 | 3.3 | SSE stream of rows, panels and busy state; the shared runner (`internal/runner`) used by the TUI and the web UI; other mrm processes noticed through their lock and the status cache |
| ✅ 2026-10-15 | 3.4 | Overview, Repo, Activity, Settings pages; long groups capped with a filter link                                                                                                      |
| ✅ 2026-10-15 | 3.5 | tests: security, pages, filter, a sync streamed over SSE on a fixture repo, history, secrets hidden, external lock; layout checked in Chrome at 1440/768/518 px                      |
| ⬜            | 3.6 | UAT walkthrough of phase 3 in your browser (light and dark, text sizes, a real sync from the page)                                                                                   |

## Phase 4 - Extras

| ✓            | #   | Step                                                                                                                                                                                                     |
| ------------ | --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-19 | 4.1 | `exec` (parallel, whole-line prefixed output, `--serial` with the terminal attached, summary of failures); asks first, refuses without a terminal unless `--yes`                                         |
| ✅ 2026-10-19 | 4.2 | `post_sync` hooks (`if_file`, only when `HEAD` moved, `MRM_*` env, 5-minute timeout; a failure is a warning on the result)                                                                               |
| ✅ 2026-10-19 | 4.3 | `prune` (merged into the remote default branch, or upstream gone; unmerged kept unless `--force`; merged re-checked before each delete; `--dry-run`)                                                     |
| ✅ 2026-10-19 | 4.4 | `schedule install/list/remove` (systemd user timers, `Persistent=true`, calendar checked with `systemd-analyze`); `doctor` reports the user instance                                                     |
| ✅ 2026-10-19 | 4.5 | forge clients in `internal/forgeapi` (Q1 closed: not shared): PR count and CI state for GitHub, GitLab, Bitbucket, Gitea in status, TUI and web, cached in `forge.json`; `drift` against config and orgs |
| ⬜            | 4.6 | release 0.2.0 (done 2026-10-19: tagged, draft release with archives); left: `go install` check on a clean machine; UAT walkthrough                                                                       |

## Phase 5 - Polish
Ideas from using 0.2.0, for 0.3.0.

| ✓            | #    | Step                                                                                                                               |
| ------------ | ---- | ---------------------------------------------------------------------------------------------------------------------------------- |
| ✅ 2026-10-22 | 5.1  | README for newcomers, user guide, pictures made from mrm's real output on a demo fleet (`task docs:images`)                        |
| ✅ 2026-10-22 | 5.2  | TUI keys in one table read by the handler, help and palette; a test fails when the guide misses a key                              |
| ✅ 2026-10-22 | 5.3  | `←` `→` cycle workspaces, `shift+tab` all groups, `J` `K` / `shift+↑↓` move 10, `p` `P` `b` open PRs, pipelines, branch (`O` gone) |
| ✅ 2026-10-22 | 5.4  | footer fits the width and follows the context                                                                                      |
| ✅ 2026-10-22 | 5.5  | `L` commit graph in a modal; `ctrl+d` `ctrl+u` scroll                                                                              |
| ✅ 2026-10-22 | 5.6  | mouse (`tui.mouse`); the view reopens where you left it                                                                            |
| ✅ 2026-10-22 | 5.7  | `ff_dirty` sync policy                                                                                                             |
| ✅ 2026-10-22 | 5.8  | docs: sync step by step with the exact git commands; compared with `git pull`                                                      |
| ⬜            | 5.9  | Bitbucket Server / Data Center: web URLs (`/projects/X/repos/Y`), PR and CI counts, `drift` - if needed                            |
| ⬜            | 5.10 | web UI keys: `/` focuses the filter, `?` lists keys                                                                                |
| ⬜            | 5.11 | release 0.3.0; UAT walkthrough of phase 5 in a real terminal                                                                       |

## Releases

| Version | Date       | Contains                                                                                                         |
| ------- | ---------- | ---------------------------------------------------------------------------------------------------------------- |
| 0.1.0   | 2026-10-15 | phases 1-2: CLI and interactive view; release archives for linux/darwin amd64/arm64 (GoReleaser, `task release`) |
| 0.2.0   | 2026-10-19 | phases 3-4: web UI; `exec`, hooks, `prune`, `drift`, `schedule`, forge PR/CI counts                              |

## Follow-Ups

| #      | Item                                                                                                                                                                                                                                                             |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| F1     | issue tracker keys from branch names (e.g. `PROJ-123`) as a column and a link - only if wanted after phase 2                                                                                                                                                     |
| F2     | show linked worktrees under their main repo                                                                                                                                                                                                                      |
| ~~F3~~ | ~~sync of 86 repos took 31.9 s against the 30 s target~~ - done 2026-10-15: `origin/HEAD` re-checked at most once a day (the ref file's age decides); the same sync now takes 15.6 s. With git >= 2.48, `remote.origin.followRemoteHEAD` could replace the check |
| ~~F4~~ | ~~`forge:` in filters ignores the `forges:` host mapping~~ - done 2026-10-15                                                                                                                                                                                     |
| ~~F5~~ | ~~status names truncate at 44 characters~~ - done 2026-10-15: sized from the terminal width; piped output is never cut                                                                                                                                           |
| F6     | before going public: rebuild history from the generic docs (done 2026-10-14), recreate the GitHub repo so no old objects survive (done 2026-10-14), tag `v0.1.0` (done 2026-10-15); then make the repo public                                                    |
| ~~F7~~ | ~~shell completion for values~~ - done 2026-10-15: `-w`, `-t`, `-f` terms and `mrm open` repo names                                                                                                                                                              |
# Specification

- [Specification](#specification)
  - [Purpose](#purpose)
  - [Scope](#scope)
  - [Architecture](#architecture)
  - [Decisions](#decisions)
  - [Concepts](#concepts)
  - [Status](#status)
  - [Sync](#sync)
  - [Filters](#filters)
  - [CLI](#cli)
  - [TUI](#tui)
  - [Web UI](#web-ui)
  - [Open in Browser](#open-in-browser)
  - [Configuration](#configuration)
  - [Notifications](#notifications)
  - [Scheduling](#scheduling)
  - [Security](#security)
  - [Non-Functional Requirements](#non-functional-requirements)
  - [Delivery Phases](#delivery-phases)
  - [Out of Scope](#out-of-scope)
  - [Open Questions](#open-questions)

## Purpose
Anyone who works across many repositories - dozens for a team's services, more again for personal projects, spread over GitHub, GitLab and Bitbucket and nested in folders by team or project - ends up keeping them current with a hand-rolled script. Those scripts tend to:

- work from a hand-kept list that drifts: repos get cloned and never added, and the branch noted for a repo stops being its default
- check out a branch and run `git pull` whatever state the repo is in
- run one repo at a time and only report failures at the end
- be tied to one folder layout, and keep notification tokens in source

**mrm** (mass repo manager) is one Go binary that **finds repos itself**, keeps them current **safely and in parallel**, and shows the state of all of them **live** - as plain output, an interactive TUI or a local web UI.

| Goal                                 | How                                                                                                           |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| no list to maintain                  | repos are **discovered** under configured roots; config only holds exceptions (branch override, tags, ignore) |
| updating never loses or tangles work | fetch always; **fast-forward only**, only when clean; never switch branch, stash, reset, rebase or merge      |
| see everything at a glance           | one status model behind the CLI, the TUI and the web UI                                                       |
| fast                                 | bounded parallel workers; a status cache so the TUI and web UI open instantly                                 |
| work found before it is lost         | unpushed commits, stashes and dirty trees flagged across every repo                                           |
| any layout                           | any number of **workspaces** (`work`, `personal`, ...), each with its own roots and sync policy               |

## Scope
| In                                                                                     | Out                                                                        |
| -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| discovering repos under roots, with ignore globs and nested group folders              | single-repo git work - staging, committing, rebasing (hand off to lazygit) |
| status of every repo: branch, ahead/behind, dirty, stashes, unpushed work, last commit | rewriting history, resolving conflicts, deleting work                      |
| safe sync (fetch + fast-forward) of all or a filtered set                              | creating PRs or other forge write operations                               |
| interactive TUI with live updates and bulk actions                                     | a hosted or multi-user web UI                                              |
| local web UI with an overview of everything                                            | native Windows (Linux, macOS and WSL only)                                 |
| opening a repo, its PRs or its CI in the browser                                       | issue tracker integration                                                  |
| running a command across repos (`exec`), post-sync hooks                               | cloning whole orgs automatically (drift is reported, not acted on)         |
| notifications after a headless sync; a scheduled sync                                  |                                                                            |
| JSON output and exit codes for scripts                                                 |                                                                            |

## Architecture
```
                  ┌──────────────────────── mrm (one binary) ────────────────────────┐
                  │                                                                  │
  mrm sync ──────►│  cli (plain / --json) ─┐                                         │
  mrm (TTY) ─────►│  tui (tview/tcell) ────┼──► engine ──► worker pool ──► git -C <repo> ...
  mrm web ───────►│  web (net/http, htmx,  │      │  events (chan)                    │
   browser ◄─SSE──│       SSE, go:embed) ──┘      │                                   │
                  │                               ├──► discover (roots, ignore, depth) │
                  │                               ├──► cache    $XDG_CACHE_HOME/mrm    │
                  │                               ├──► history  $XDG_STATE_HOME/mrm    │
                  │                               └──► notify   Gotify / ntfy          │
                  │  config  $XDG_CONFIG_HOME/mrm/config.yaml                         │
                  └──────────────────────────────────────────────────────────────────┘
```

The **engine** is the only thing that runs git. Front ends send it requests (status, fetch, sync, exec) and render the **events** it emits (`started`, `progress`, `done`, `skipped`, `failed`), so a sync started in the TUI shows live in an open web UI too.

## Decisions
Recorded so they are not argued again. Revisit only if the reason stops holding.

| Topic                                 | Decision                                                                                                                                                                                             | Why                                                                                                                                                                           | Considered                             |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| Home                                  | its **own repo** `adegoodyer/mrm` (decided 2026-10-14)                                                                                                                                               | config, TUI, web server and releases make it a product, not a script                                                                                                          | a script in a dotfiles or scripts repo |
| Language                              | **Go**, one static binary                                                                                                                                                                            | fast, single binary, easy to install                                                                                                                                          | Python, Rust                           |
| Stack                                 | **cobra, adrg/xdg, zap, yaml.v3**; plain ANSI for CLI colour                                                                                                                                         | small, well-known dependencies                                                                                                                                                | kong, urfave/cli                       |
| No viper                              | config decoded with **yaml.v3 into typed structs**, strict (unknown keys are errors); mrm never rewrites the file (decided 2026-10-14)                                                               | viper lowercases keys and splits them on `.`, which breaks repo keys that are paths and globs; writing the file back would drop comments                                      | viper                                  |
| TUI library                           | **tview on tcell**, never Bubble Tea (decided 2026-10-14/15); the repo list is a custom tview primitive, overlays use tview's input, list and pages                                                  | maintainer preference                                                                                                                                                         | Bubble Tea                             |
| Git access                            | **shell out to `git -C <repo>`**, `GIT_TERMINAL_PROMPT=0`, `ssh -o BatchMode=yes`, a timeout per repo                                                                                                | uses the ssh-agent, credential helpers and config already set up; nothing can hang on a prompt                                                                                | go-git (slower, auth is separate work) |
| Repo list                             | **discovered**, not declared; config holds exceptions only                                                                                                                                           | hand-kept lists drift: repos get cloned and never added                                                                                                                       | a declared list                        |
| Default branch                        | from **`origin/HEAD`**, **re-checked by fetch at most once a day** (`git remote set-head --auto`; the ref file's age decides), overridable per repo; fall back to `main` then `master` on the remote | `origin/HEAD` is set at clone time and goes stale when a forge's default changes (seen on real repos: `develop` locally, `master` on the forge); `doctor` flags a missing one | always configured                      |
| Sync policy                           | **fetch + fast-forward only**, only when clean; never switch branch, stash, reset, rebase or merge by default                                                                                        | a sync must never change work in progress                                                                                                                                     | checkout + pull                        |
| Default branch when on another branch | updated in place from the fetched remote ref with `git fetch . origin/<b>:<b>` (git refuses a non-fast-forward)                                                                                      | the feature branch is untouched and the default is still current for the next `switch`                                                                                        | switching to it and back               |
| Status priority                       | one **primary state** per repo by precedence ([Status](#status)) plus flags                                                                                                                          | a list needs one glyph per row; the flags carry the detail                                                                                                                    | many columns of booleans               |
| Concurrency                           | bounded worker pool, default **8**, one sync per workspace at a time (lock file)                                                                                                                     | the network is the bottleneck; a timer and a manual run must not collide                                                                                                      | unbounded goroutines                   |
| Cache                                 | last status per repo in `$XDG_CACHE_HOME/mrm/status.json`                                                                                                                                            | TUI and web UI paint at once, then refresh                                                                                                                                    | no cache                               |
| TUI                                   | a **fleet view**: one dense, grouped, live list with a command palette and filters; `enter` hands off to lazygit                                                                                     | lazygit already does one repo well; mrm is the view across all of them                                                                                                        | lazygit-style fixed panels             |
| Web UI                                | **Go templates + htmx + SSE**, hand-written CSS, embedded with `go:embed`                                                                                                                            | no Node toolchain; live updates with no client framework                                                                                                                      | SPA (React/Svelte)                     |
| Web access                            | bind **127.0.0.1** only, random token per run, Host header check                                                                                                                                     | it can run git in every repo                                                                                                                                                  | open on the LAN                        |
| Config format                         | **YAML**                                                                                                                                                                                             | the common config language of dev tooling (Taskfile, Helm, Kubernetes)                                                                                                        | TOML                                   |
| Secrets                               | never in config: `*_cmd` (e.g. `pass show ...`) or `*_env` only                                                                                                                                      | config files get copied and committed                                                                                                                                         | literal tokens                         |
| No config                             | the current directory is the only root (depth 3)                                                                                                                                                     | useful on first run and on other machines                                                                                                                                     | refuse to run                          |
| Name                                  | **mrm** (decided 2026-10-14)                                                                                                                                                                         | short; an npm package of the same name exists but only matters if both are on `PATH`                                                                                          | gitfleet, herd                         |

## Concepts
| Term      | Meaning                                                                                                                                     |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| workspace | a named set of roots with its own sync policy, e.g. `work` (`~/work`) and `personal` (`~/src`)                                              |
| root      | a folder scanned for repos, down to `depth` (default 3)                                                                                     |
| repo      | a folder containing `.git`. Discovery **does not descend into a repo**, so submodules and `.worktrees/` inside it are not listed separately |
| group     | the folders between the root and the repo (`platform` for `~/work/platform/billing-api`); empty for top-level repos                         |
| tag       | free labels from config (`backend`, `terraform`), matched by repo name or path glob                                                         |
| forge     | detected from the remote URL: GitHub, GitLab, Bitbucket, Gitea/Codeberg, or unknown                                                         |

Folders under a root that are not repos (`~/work/logs`, `~/work/scratch`) are ignored silently; group folders are walked.

## Status
Read with `git status --porcelain=v2 --branch --show-stash` plus `for-each-ref` for unpushed branches. No network unless fetching.

| Field        | Detail                                                                                                                        |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| identity     | name, path, workspace, group, tags, remote URL, forge                                                                         |
| branch       | current branch (or detached), default branch, upstream, ahead/behind upstream, default branch ahead/behind `origin/<default>` |
| working tree | staged, modified, untracked, conflicted counts; merge/rebase/cherry-pick in progress                                          |
| work at risk | stash count; unpushed commits (on branches never pushed, or ahead of a live upstream); stale branches (upstream gone)         |
| history      | last commit time, author, subject; last fetch time                                                                            |
| result       | last sync outcome and reason                                                                                                  |

**Primary state**, first match wins. Glyphs are single-width in common terminal fonts so columns stay aligned (an emoji like `⚡` renders double-width):

| Glyph | State    | When                                                                                            |
| ----- | -------- | ----------------------------------------------------------------------------------------------- |
| `✗`   | error    | fetch failed, not a repo any more, timeout                                                      |
| `≠`   | conflict | merge, rebase, cherry-pick or revert in progress                                                |
| `⇅`   | diverged | ahead and behind its upstream                                                                   |
| `●`   | dirty    | staged, modified or conflicted files (untracked alone is a flag, not dirty)                     |
| `↑`   | unpushed | commits only here: on a branch that was never pushed, or ahead of an upstream that still exists |
| `↓`   | behind   | behind upstream, or default branch behind `origin/<default>`                                    |
| `✓`   | clean    | none of the above                                                                               |

Flags shown alongside: `stash`, `untracked`, `detached`, `no-remote`, `upstream-gone`, `no-upstream`, `not-default` (on a branch other than the default), `stale-branches`.

A branch whose upstream is gone - deleted on the forge after a merge, then pruned by a fetch - is **stale**, not unpushed: its commits were pushed once. Counting them as unpushed flagged 28 of 86 real repos as work at risk after the first `fetch --prune`, all of them old `develop` branches. Stale branches are a flag (listed by `prune` in phase 4); a stale branch with commits added after its last push can't be told apart once pruned.

## Sync
Per repo, in the worker pool:

| #   | Step                                                                                                                                                                                                                                                      | Skips with reason                                            |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| 1   | `git fetch --prune origin` (remote configurable), then `git remote set-head origin --auto` if `origin/HEAD` was last checked over a day ago (a second network round trip; every time, it doubled sync time) - not when the branch is overridden in config | `error: <git message>` on failure or timeout                 |
| 2   | conflict in progress                                                                                                                                                                                                                                      | `conflict` - nothing else runs                               |
| 3   | on the default branch: if strictly behind and the tracked files are clean, `git merge --ff-only @{u}`                                                                                                                                                     | `dirty`, `diverged`                                          |
| 4   | on another branch: update the default branch in place with `git fetch . origin/<default>:<default>` (local, no second network trip)                                                                                                                       | `default diverged`                                           |
| 5   | on another branch with `ff_current: true`: fast-forward it too if clean and strictly behind its upstream                                                                                                                                                  | `dirty`, `diverged`                                          |
| 6   | with `switch_merged: true`: if clean and the current branch was pushed (has an upstream, even a gone one) and is fully merged into the default, switch to the default - before steps 3-5                                                                  | - (stays put)                                                |
| 7   | run matching `post_sync` hooks if `HEAD` moved                                                                                                                                                                                                            | `hook failed: <name>` (the sync itself still counts as done) |

mrm never stashes, resets, rebases, creates a merge commit, force-anything or deletes a branch during sync. Untracked files don't count as dirty; a fast-forward is only refused (with git's message) if it would overwrite one. Detached `HEAD` gets step 1 only.

After a run: a summary (synced, already current, skipped by reason, failed), each run appended to `$XDG_STATE_HOME/mrm/runs.jsonl`, notifications ([Notifications](#notifications)). Exit code `0` all synced or current, `2` some repos need attention, `1` mrm itself failed.

## Filters
One query language in the CLI (`--filter`), the TUI (`/`) and the web UI (search box). Terms are ANDed; a bare word matches part of the name or path (substring, not fuzzy - an unranked filter has to be predictable; `web` must not match `wallet-backend`). Ranked fuzzy search belongs to the TUI palette.

| Term                                        | Matches                                                                   |
| ------------------------------------------- | ------------------------------------------------------------------------- |
| `is:<state or flag>`                        | `is:dirty`, `is:behind`, `is:unpushed`, `is:stash`, `is:not-default`, ... |
| `ws:<name>` · `group:<name>` · `tag:<name>` | workspace, group, tag                                                     |
| `branch:<glob>` · `forge:<name>`            | current branch, forge                                                     |
| `age:>30d` · `age:<1d`                      | last commit age                                                           |
| `-<term>`                                   | negation, e.g. `-tag:archived`                                            |

## CLI
Global flags: `-w/--workspace`, `-t/--tag`, `-f/--filter "<query>"`, `--json`, `--no-color` (and `NO_COLOR`), `-c/--config <path>`, `-v/--verbose`.

| Command                                  | Does                                                                                                                  |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `mrm`                                    | the TUI on a terminal, otherwise `mrm status`                                                                         |
| `mrm status`                             | one line per repo (glyph, name, branch, ahead/behind, flags, age); `--json` for scripts; `--fetch` to fetch first     |
| `mrm sync`                               | headless fetch + fast-forward with a live progress line per repo, then the summary; `--dry-run` says what it would do |
| `mrm fetch`                              | step 1 of sync only                                                                                                   |
| `mrm web`                                | serve the web UI; `--open` opens the browser, `--port` (default 7420)                                                 |
| `mrm open <repo> [--pr\|--ci\|--branch]` | the repo, its PRs, its CI or the current branch on the forge                                                          |
| `mrm exec -- <cmd>`                      | run a command in every matching repo, in parallel, output prefixed by repo; `--serial` for interactive commands       |
| `mrm prune`                              | list local branches already merged or with a gone upstream; delete only after confirmation                            |
| `mrm drift`                              | repos on disk that config refers to but are missing, and (with a forge token) repos in an org that are not cloned     |
| `mrm schedule install\|list\|remove`     | a systemd user timer for `mrm sync` ([Scheduling](#scheduling))                                                       |
| `mrm config init\|edit\|path`            | write a commented default, open it in `$EDITOR`, print the paths                                                      |
| `mrm doctor`                             | git version, config valid, roots exist, repos with no `origin/HEAD`, ssh agent loaded, config permissions             |
| `mrm version`                            | version, commit, build date                                                                                           |

Every command that changes something outside `.git` (`prune`, `schedule`, `exec`) asks first unless `--yes`.

## TUI
`mrm` on a terminal (stdin and stdout both TTYs, no `--json`); piped, it prints `mrm status`. `-w <one workspace>` and `-f` / `-t` set the starting view. tview on tcell (decided 2026-10-15). The list is a custom widget drawn cell by cell, so group headers span the full width and every column truncates with `…`. Opens from the cache at once (first frame 13 ms for 86 repos), then refreshes status in the background (no fetch unless `tui.fetch_on_start: true`).

```
 mrm  all · 78 repos    ● 2  ↑ 1  ↓ 3  ✓ 71  ✗ 1                     synced 4m ago   ctrl+k
─ ▾ services · 3 ─────────────────────────────────────────────────────────────────────────
  ✓  api                 develop    ·          2h  fix: rounding in invoices
▌ ●  web                 feature/x  2↑   3M    1d  wip: checkout layout
  ⠋  docs                main       ·          5d  syncing…
─ ▾ platform · 1 ─────────────────────────────────────────────────────────────────────────
  ↓  billing-api         master     4↓         3d  chore: bump deps
─ ▸ terraform · 6 ────────────────────────────────────────────────────────────────────────
 1 marked  s sync  f fetch  A clear marks  ctrl+k more
```

- **Rows**: mark (`▌`), state glyph (a spinner while an operation runs), name, branch, ahead/behind, changes, last commit age, then the outcome of the last run if it needs attention, else notes, else the last commit subject.
- **Groups**: workspace, then group folder (`work · platform` when more than one workspace is shown); `▸` collapsed, `▾` open, with a count. The cursor can sit on a header: actions there apply to the whole group.
- **Targets**: an action applies to the marked repos, else the group under the cursor, else the repo under it. Marks clear once an action starts.
- **Operations** run one at a time through the engine, streaming a live row per repo; a sync takes the same workspace lock as `mrm sync` and writes the same cache and run history. A sync that leaves anything skipped or failed ends on the needs-attention view.
- **Quit** while an operation runs needs `q` twice; git finishes what it started.

| Key                            | Action                                                                                                             |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `j/k` `↑/↓` `g/G` `PgUp/PgDn`  | move                                                                                                               |
| `tab` (or `enter` on a header) | collapse / expand the group                                                                                        |
| `/`                            | filter as you type ([Filters](#filters)); `enter` keeps it, `esc` clears it                                        |
| `ctrl+k`                       | command palette - ranked fuzzy search over every action, workspace and common filter                               |
| `space` · `a` · `A`            | mark the repo (or the whole group on a header), mark everything shown, clear marks                                 |
| `s` · `S`                      | sync the targets / everything shown                                                                                |
| `f` · `F`                      | fetch the targets / everything shown                                                                               |
| `r`                            | refresh status                                                                                                     |
| `o` · `O`                      | open on forge / open PRs (the palette has CI and branch too)                                                       |
| `enter`                        | hand off to `tui.open_cmd` (default `lazygit`) in the repo; the row refreshes on return                            |
| `e` · `t`                      | `$VISUAL`/`$EDITOR` / `$SHELL` in the repo                                                                         |
| `l`                            | detail pane: path, forge URL, upstream, default branch, last run, branches with tracking, stashes, last 12 commits |
| `!`                            | needs-attention view: everything not clean, grouped by reason - failures first, then skips, then states            |
| `w`                            | switch workspace                                                                                                   |
| `esc`                          | clear the filter, else leave the attention view, else clear marks                                                  |
| `?` · `q`                      | help, quit                                                                                                         |

Colours are the terminal's own ANSI palette on its default background, so light and dark themes both read; tcell drops colour under `NO_COLOR` (or `--no-color`) and bold, dim and reverse still carry the meaning.

## Web UI
`mrm web` serves `http://127.0.0.1:7420/?token=<random>`, printed and opened with `--open`. Go templates + htmx, live updates over SSE, hand-written CSS with light/dark/system themes, responsive from 360 px.

| Page                | Shows                                                                                                                                                                                     | Actions                                           |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| **Overview** (home) | state counts per workspace; **needs attention** list; **work at risk** (unpushed commits, stashes, dirty trees older than a day); filterable table of every repo; last sync per workspace | sync / fetch all, a workspace or the filtered set |
| **Repo**            | status, branches with ahead/behind, stashes, last 20 commits, last sync results, forge links                                                                                              | sync, fetch, open on forge                        |
| **Activity**        | sync runs from `runs.jsonl`: when, what, duration, outcome per repo                                                                                                                       | -                                                 |
| **Settings**        | config as loaded, its path and validation errors                                                                                                                                          | - (edit with `mrm config edit`)                   |

The web UI offers only what the engine offers - no `exec`, no hooks, no `prune`.

## Open in Browser
The web URL is derived from the remote (`git@host:owner/repo.git` or `https://...`); hosts mapped to a forge in config cover self-hosted ones.

| Forge            | PRs                 | CI             |
| ---------------- | ------------------- | -------------- |
| GitHub           | `/pulls`            | `/actions`     |
| GitLab           | `/-/merge_requests` | `/-/pipelines` |
| Bitbucket        | `/pull-requests`    | `/pipelines`   |
| Gitea / Codeberg | `/pulls`            | `/actions`     |

The browser is opened with `browser_cmd` if set, else `wslview` on WSL, else `xdg-open` / `open`.

## Configuration
| What             | Path                                                             |
| ---------------- | ---------------------------------------------------------------- |
| config           | `$XDG_CONFIG_HOME/mrm/config.yaml` (`~/.config/mrm/config.yaml`) |
| run history, log | `$XDG_STATE_HOME/mrm/runs.jsonl`, `mrm.log`                      |
| status cache     | `$XDG_CACHE_HOME/mrm/status.json`                                |
| sync lock        | `$XDG_RUNTIME_DIR/mrm/<workspace>.lock` (state dir if unset)     |

Precedence: flags > `MRM_*` environment > config > defaults.

```yaml
concurrency: 8
timeout: 2m                       # per repo, per git command
browser_cmd: ""                   # default: wslview / xdg-open / open

workspaces:
  work:
    roots: [~/work]
    depth: 3
    ignore: [legacy-*, "*-archive"]
    sync:
      ff_current: false
      switch_merged: false
  personal:
    roots: [~/src]

repos:                            # exceptions only; keys are names or path globs
  payments-service:
    branch: develop               # overrides origin/HEAD
    tags: [backend]
  "~/work/terraform-*":
    tags: [terraform]
  "~/work/legacy-portal":
    sync: false                   # status only

hooks:
  post_sync:
    - name: pre-commit
      if_file: .pre-commit-config.yaml
      run: pre-commit install --install-hooks

notify:
  on: attention                   # attention | always | never
  gotify:
    url: <gotify-url>
    token_cmd: pass show gotify/mrm
  ntfy:
    url: <ntfy-url>/<topic>
    token_env: MRM_NTFY_TOKEN

tui:
  open_cmd: lazygit
  fetch_on_start: false

web:
  port: 7420
```

`mrm config init` writes this file commented, with one workspace per common clone folder that exists (`~/src`, `~/code`, `~/projects`, `~/repos`, `~/work`, `~/dev`, `~/git`), named after the folder - or the current directory if none do.

## Notifications
After a headless `mrm sync` (also from the timer), per `notify.on`: `attention` sends only when something was skipped or failed, `always` sends every run. One message per run: counts, then the names needing attention with their reason. Gotify and ntfy in phase 1; desktop (`notify-send`) later if wanted.

## Scheduling
`mrm schedule install -w work --on "Mon..Fri 08:30"` writes `mrm-sync-work.service` and `.timer` to `~/.config/systemd/user/` with `Persistent=true` (catches up after the machine was off) and enables it. `schedule list` and `schedule remove` manage them. Needs systemd (on WSL, enable it in `/etc/wsl.conf`); `doctor` says when it is not available.

## Security
| Entry point / asset                  | Risk                                                         | Mitigation                                                                                                                                                                                  |
| ------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| web UI                               | another local process or a web page drives git in every repo | 127.0.0.1 only; random token per run (cookie after first load); `Host` must be `127.0.0.1`/`localhost` (DNS rebinding); POSTs need the token header (CSRF); no `exec` or hooks from the web |
| secrets (Gotify, ntfy, forge tokens) | leaked from config or logs                                   | only `*_cmd` / `*_env` fields exist; never logged; redacted in `--verbose` and the Settings page                                                                                            |
| git auth                             | a prompt hangs a worker or a run                             | `GIT_TERMINAL_PROMPT=0`, `GIT_SSH_COMMAND` with `BatchMode=yes`, timeout per command                                                                                                        |
| config                               | hooks and `exec` run commands from it                        | the config is trusted input; `doctor` warns if it is group- or world-writable                                                                                                               |
| working trees                        | a sync loses or tangles work                                 | the [sync rules](#sync); tests prove a dirty tree is byte-identical after a sync                                                                                                            |

## Non-Functional Requirements
| Requirement                           | Target                                                             |
| ------------------------------------- | ------------------------------------------------------------------ |
| status of 130 repos, no fetch         | < 1 s warm, < 3 s cold                                             |
| sync of 80 repos with nothing to pull | < 30 s at concurrency 8 (network-bound)                            |
| TUI first paint                       | < 150 ms (from cache)                                              |
| no lost work                          | no command changes a dirty tree or rewrites history (tested)       |
| no hangs                              | every git call has a timeout; no prompt can block                  |
| platforms                             | Linux and macOS, amd64 and arm64; WSL                              |
| install                               | `go install github.com/adegoodyer/mrm@latest`, or a release binary |

## Delivery Phases
| Phase               | Delivers                                                                                                     |
| ------------------- | ------------------------------------------------------------------------------------------------------------ |
| 1 - core + headless | config (XDG), discovery, status, sync, fetch, open, filters, `--json`, cache, history, Gotify/ntfy, `doctor` |
| 2 - TUI             | the fleet view, palette, filters, marks, bulk actions, hand-off, needs-attention view                        |
| 3 - web UI          | Overview, Repo, Activity, Settings, live over SSE                                                            |
| 4 - extras          | `exec`, hooks, `prune`, `drift`, `schedule`, forge PR/CI counts                                              |

## Out of Scope
- single-repo git operations (lazygit does it)
- cloning, archiving or creating repos on a forge
- a hosted or shared web UI
- issue trackers (branch-name ticket keys may come later)
- native Windows

## Open Questions
| #      | Question                                                                                                                    | Leaning                                                                                                                     |
| ------ | --------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Q1     | forge PR/CI counts (phase 4) need an API token per forge. Share forge clients with the maintainer's repo-stats as a module? | yes, extract a shared module when phase 4 starts                                                                            |
| ~~Q2~~ | ~~TUI library~~                                                                                                             | closed 2026-10-15: tview, with the list drawn as a custom primitive (tview's table can't span group headers across columns) |
| Q3     | should `switch_merged` default to true?                                                                                     | decide after a few weeks of phase 1 use                                                                                     |

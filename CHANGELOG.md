# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `d` in the interactive view moves the marked repos (or the group or repo under the cursor) to the trash after one confirmation, which flags any with work that exists only on this machine. They can be restored from the trash. Workspace roots and folders that aren't git repos are never touched.
- The user guide spells out sync step by step - the exact git commands, in order - and compares it with `git pull` when you have local changes.
- `ff_dirty: true` under a workspace's `sync:` fast-forwards a repo with uncommitted changes when the update changes none of the same files, keeping the edits. When it would touch one, the repo is skipped and the reason names the files. Off by default: a dirty repo is still skipped.
- `L` in the interactive view shows the repo's commit graph - every branch, tag and remote, newest first, in git's colours - in a scrollable window, without any extra tool. It reads at most the newest 300 commits and says when there are more.
- The interactive view takes the mouse: click a row to select it (a second click on a group header collapses it) and scroll with the wheel. `tui.mouse: false` turns it off.
- The interactive view reopens where you left it: the same workspace, collapsed groups and detail pane, from `~/.local/state/mrm/tui.json`. `-w` still picks the workspace.
- `ctrl+d` / `ctrl+u` scroll the detail pane and the graph half a page; with the pane closed they move the list.
- **User guide** (`docs/user-guide.md`): concepts (workspaces, discovery, groups, tags, the default branch, every state, flag and outcome), every config key with its default, status, sync rules and policies, filters, the interactive view's keys, the web UI and its security, `open`, `exec`, `prune`, `drift`, forge tokens per forge, hooks, schedules, notifications, scripting, completion, troubleshooting and uninstalling.
- Pictures in the README and the guide - `mrm status`, `mrm sync`, the interactive view and its detail pane, the web UI in light and dark, how mrm fits together, and what sync does to each repo. The terminal pictures are mrm's real output on a demo fleet, rebuilt with `task docs:images`.
- `group:` in a repo's settings puts it in a group by name pattern, without moving folders: `"inf-*": { group: infra }`. Like `branch:`, the most specific key wins. A repo has one group and any number of tags.
- The user guide starts its concepts with one table of the terms - workspace, group, tag, VCS host, root - and what each is for.

### Changed
- "Forge" is gone from what you see: it's a **VCS host** (GitHub, GitLab, Bitbucket, Gitea), and screens name the actual site - the web Repo page says "Open on GitHub". The config follows: `hosts:`, `host_tokens:`, `host_cache:` and the `host:` filter replace `forges:`, `forge_tokens:`, `forge_cache:` and `forge:`. The old names still work, and `mrm doctor` suggests the new ones. `host:` also takes a hostname (`host:git.example.com`). `mrm status --no-prs` replaces `--no-forge` (still accepted), and the PR/CI cache is now `~/.cache/mrm/hosts.json`. The `forge` field in `--json` output is unchanged.
- In the interactive view, `o` `p` `P` `b` open a page for every marked repo (or every repo in the group under the cursor), asking first when that's more than 5 tabs. Before, they opened only the repo under the cursor. Questions like that one appear in mrm's own dialog: `y` to go ahead, `n` or `esc` to cancel.
- Sync results read as sentences: `fast-forwarded main by 3 commits` and, in a dry run, `would fast-forward main by 3 commits` (was `main +3` and `would main +3`); a branch switch reads `switched from feature/x to main (merged)`.
- The web UI uses the full width of the window instead of a centred 1240 px card, so wide screens show more of each repo's notes. Phones get the same stacked layout as before.
- Interactive view keys:
  - `←` / `→` step through the workspaces, with *all* as one of them.
  - `shift+tab` collapses every group, or expands them all when all are collapsed.
  - `J` / `K` and `shift+↓` / `shift+↑` move 10 rows.
  - `p` opens the repo's pull requests, `P` its pipelines or actions, and `b` the current branch on the forge. `O` is gone: `p` replaces it, so an uppercase key now always means "everything shown" or a sibling page.
- The interactive view's footer shows as many keys as fit the terminal, most useful first, always ending with `? help`. It follows what you're doing: on a group header it shows the group keys, with repos marked the keys for them, and with a filter on, the filter and how to clear it.
- The interactive view's keys come from one table, so `?` help, the command palette and the docs always list the same keys. Help scrolls when the terminal is short and closes with `esc`, `q` or `?`. `Home` and `End` (top and bottom) were already there; they are now documented.
- The README is rewritten for someone new to mrm: what it is and why, install, a five-minute first run, how to read the status, then each way in, with links into the guide.
- Syncs and fetches start with the repos that were slowest last time, so a slow VCS host overlaps the quick ones instead of finishing alone at the end. `mrm status --json` gains `fetch_error` and `fetch_ms`.
- A fetch that is going nowhere gives up sooner. The default `timeout` per git command is 30 s, down from 2 minutes, and a fetch over ssh now fails after 10 s when the host doesn't answer, or 15 s after a connection stops answering. A fetch over https that stalls (under 1 KB/s for 15 s) fails too. A config written by an older `mrm config init` still says `timeout: 2m` until you change it.
- Repos in no group are listed after the groups, under *(no group)* in the interactive view, instead of under a heading that repeated the workspace's name. A workspace with no groups is still one section named after it.
- `mrm doctor` and `mrm schedule install` warn when systemd *linger* is off for you: without it, timers stop when your last terminal closes. The fix is in the warning (`sudo loginctl enable-linger $USER`).

### Fixed
- A repo deleted while the interactive view or the web UI is open no longer stays listed as `missing` until restart. `r` in the interactive view, and each load of the web Overview, look for repos again: deleted ones drop off and repos cloned since appear.
- A failed fetch is remembered with the repo's status, so it stays `error` (`✗`, `is:error`, the web UI) with `last fetch failed: …` until a fetch works again. Before, the next status read - starting the interactive view, say - made it look fine. The spec and the guide already described it this way.
- `mrm status` or `mrm sync` with a filter that matches nothing says "no repos match the filter" (or prints `[]` with `--json`) and exits 0. Before, it claimed no repos were found under the roots and exited 1, like a bad config.

## [0.2.0] - 2026-10-19

The web UI (phase 3) and the extras (phase 4): `exec`, hooks, `prune`, `drift`, scheduled syncs, and pull request and CI counts from the forges.

### Added
- **Web UI**: `mrm web` (`--open`, `--port`, `-w`) serves an overview of every repo on 127.0.0.1 - a state bar per workspace, what needs attention and what is at risk (unpushed commits, stashes, long-dirty trees, stale branches), and every repo, filtered as you type with the same language as `-f`. A page per repo (branches, stashes, last 20 commits, forge links), the run history, and settings: text size (Small to Extra large) and theme per browser, the config with secrets hidden. Sync or fetch a repo, a workspace, the filtered set or everything; rows update live in every open tab, and syncs run by other mrm processes show while they run.
- Web security: this machine only, a new access token each run (kept in an HttpOnly cookie), Host check against DNS rebinding, a token header on every action, and a strict content security policy - no inline or third-party code. Fonts are embedded, so it works offline.
- `web.text_size` in the config: the default text size for every browser.
- **`mrm exec -- <cmd>`**: run a command in every matching repo, in parallel with each output line prefixed by its repo, or `--serial` with the terminal attached; ends with the repos where it failed (exit 2).
- **Hooks**: `hooks.post_sync` commands run after a sync moved `HEAD`, in repos that have `if_file`, with `MRM_REPO`, `MRM_REPO_NAME`, `MRM_WORKSPACE` and `MRM_BRANCH` set and a 5-minute timeout. A failing hook is a warning on the repo, not a failed sync.
- **`mrm prune`**: list local branches merged into the remote default branch, or whose upstream is gone, and delete the merged ones after one confirmation. Unmerged gone-upstream branches are kept unless `--force`; the current and default branches are never touched; `--dry-run` lists only.
- **`mrm schedule install|list|remove`**: a systemd user timer running `mrm sync` (`--on "Mon..Fri 08:30"`, checked with `systemd-analyze`), catching up after the machine was off.
- **Pull requests and CI**: with read-only tokens under `forge_tokens` (GitHub, GitLab, Bitbucket, Gitea/Codeberg), open PR counts and a failing or running CI on the default branch show in `mrm status` (and its `--json`), the interactive view's rows and detail pane, and the web UI's rows and Repo page. Answers are cached for `forge_cache` (default 10m) in `$XDG_CACHE_HOME/mrm/forge.json`; hosts without a token are never contacted; `mrm status --no-forge` skips them.
- **`mrm drift`**: exact `repos:` keys that match no repo on disk, and the repos in each workspace's `orgs` with no clone anywhere, each with a `git clone` command. Archived repos are counted, and listed with `--archived`. Reports only; exit 2 on drift.
- `exec`, `prune` and `schedule` ask before acting; `--yes` skips the question, and without a terminal they refuse instead of guessing.

### Changed
- The TUI runs its operations through the same runner as the web UI (lock, cache, history), so both behave identically.
- `mrm doctor` also checks that forge tokens resolve and that a systemd user instance is available for schedules.
- `mrm config path` lists the forge cache; `mrm config init` shows `orgs`, `hooks`, `forge_tokens` and `forge_cache`, commented.
- `mrm status --json` and `mrm sync --json` include each repo's warnings (from hooks).

## [0.1.0] - 2026-10-15

First release: the CLI (phase 1) and the interactive view (phase 2). Linux and macOS (amd64, arm64), including WSL.

### Added
- Spec, user acceptance tests and roadmap (`docs/`).
- Release archives for linux/darwin amd64/arm64 built with GoReleaser (`task release:snapshot`, `task release`); `mrm version` reports the tag, commit and build date.
- **Discovery**: repos are found under each workspace's roots (depth limit, ignore globs, hidden folders skipped); nested group folders (`platform/`, `services/`) become a repo's group; the walk stops at a repo, so submodules and `.worktrees/` aren't listed separately. With no config, the current directory is the only root.
- **Config** at `$XDG_CONFIG_HOME/mrm/config.yaml`: workspaces with their own sync policy, per-repo exceptions keyed by name, name glob or path glob (branch override, tags, `sync: false`), notify, forges. Read with `yaml.v3` into typed structs and never rewritten; strict - unknown keys are errors and literal secrets are refused with a hint. `mrm config init` writes a commented default with a workspace per common clone folder that exists (`~/src`, `~/code`, `~/projects`, `~/repos`, `~/work`, `~/dev`, `~/git`); `config edit` validates after saving; `config path` prints every XDG path.
- **Interactive view**: `mrm` on a terminal opens a grouped, live list of every repo (tview). Paints from the cache at once, then refreshes; `/` filters as you type, `space`/`a` mark, `s`/`f` sync or fetch the marked repos (or the group or repo under the cursor) with a live row each, `!` shows what needs attention grouped by reason, `l` a detail pane (branches, stashes, commits), `w` switches workspace, `ctrl+k` a fuzzy command palette, `enter` hands off to lazygit and back. Same engine, lock, cache and history as the CLI; works at 80 x 24 and under `NO_COLOR`.
- **`mrm status`**: one line per repo - state glyph, branch, ahead/behind, changes, last commit age, flags (stash, untracked, detached, no-remote, upstream-gone, no-upstream, stale-branches, unpushed count, default branch behind). `--fetch` to fetch first, `--json` for scripts. Name and branch columns sized from the terminal width; piped output is never cut. Single-width glyphs (`≠` conflict, `⇅` diverged) so columns stay aligned. Unpushed counts only work that exists only here - commits on never-pushed branches or ahead of a live upstream; a branch whose upstream was deleted and pruned is a stale branch instead.
- **`mrm sync`**: fetch, then fast-forward the default branch only when the tree is clean and the branch is strictly behind; from another branch, the default branch is updated in place and the current branch is left alone. Optional `ff_current`, and `switch_merged` (only away from a branch that was pushed - a brand-new branch is also "merged"). Never stashes, resets, rebases, merges or forces. Live line per repo, a summary with a needs-attention list, `--dry-run`, and exit codes `0` / `2` (needs attention) / `1` (mrm failed). One sync per workspace at a time (lock file).
- **`mrm fetch`**: fetch only. A fetch also re-checks `origin/HEAD` (`git remote set-head --auto`) when it was last checked over a day ago: it is set at clone time and goes stale when a forge's default branch changes, and checking it on every fetch doubled sync time. A sync of 86 repos takes about 15 s.
- **Filters** (`-f`, `-w`, `-t`) shared by every command: `is:<state|flag>`, `ws:`, `group:`, `tag:`, `branch:<glob>`, `forge:`, `age:>30d`, words matching part of the name or path (substring, not fuzzy), `-` to negate. `forge:` honours the `forges:` mapping for self-hosted hosts.
- **`mrm open [repo]`**: the repo, `--pr`, `--ci` or `--branch` on GitHub, GitLab, Bitbucket or Gitea/Codeberg, worked out from the remote; `--print` for the URL; `wslview` on WSL. Self-hosted hosts map to a forge under `forges:`.
- **Notifications** after a headless sync: Gotify and ntfy, `notify.on: attention | always | never`; tokens only from `token_cmd` or `token_env`, sent in headers.
- **`mrm doctor`**: git version, config validity and permissions, roots, repos with no `origin/HEAD`, broken repos, ssh agent, notify endpoints, browser opener.
- **Shell completion**: `mrm completion bash|zsh|fish|powershell`, with install steps in the README. Completes subcommands, flags and their values - workspaces (`-w`), tags (`-t`), filter terms (`-f`) and repo names (`mrm open`).
- Status cache (`$XDG_CACHE_HOME/mrm/status.json`), run history (`$XDG_STATE_HOME/mrm/runs.jsonl`) and a JSON log (`mrm.log`).
- Git is run with prompts off (`GIT_TERMINAL_PROMPT=0`, ssh `BatchMode`), a per-command timeout and a process-group kill, so nothing can hang; remote failures get a plain reason ("remote repo not found, or no access", "ssh key rejected", "offline?").
- Tests on real git fixtures (`internal/gittest`, isolated from the developer's git config) for every sync rule and nasty case; `task check` runs gofmt, vet and race tests.

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

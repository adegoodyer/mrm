# mrm

Mass repo manager - find every git repo under your folders, keep them all current safely and in parallel, and see the state of all of them at once.

> **Status:** 0.1.0 has the CLI and the interactive view; the web UI is on `next-release` for the next release. Extras (`exec`, hooks, `prune`, scheduling) come later - see the [roadmap](docs/03-roadmap.md).

- [mrm](#mrm)
  - [Install](#install)
  - [Quick Start](#quick-start)
  - [Interactive View](#interactive-view)
  - [Web UI](#web-ui)
  - [What Sync Does](#what-sync-does)
  - [Filters](#filters)
  - [Shell Completion](#shell-completion)
  - [Config](#config)
  - [Docs](#docs)
  - [License](#license)

## Install
```bash
go install github.com/adegoodyer/mrm@latest
# or from a checkout
task install   # go install . (task build for ./bin/mrm)
```

Release archives for Linux and macOS (amd64, arm64) are attached to each [GitHub release](https://github.com/adegoodyer/mrm/releases). Windows is not supported; use WSL.

## Quick Start
```bash
mrm config init            # commented config with a workspace per clone folder found (~/src, ~/code, ~/work, ...)
mrm doctor                 # check git, config, roots, repos, ssh
mrm                        # interactive view (on a terminal)
mrm web --open             # web UI in your browser
mrm status -w work         # one line per repo
mrm sync --dry-run -w work # what sync would do
mrm sync -w work           # fetch + fast-forward what is safe to
mrm open api --pr          # api's pull requests on its forge
```

With no config, the current directory is the only root.

```
↓ api                     main     ·        2d  on main, default develop 12↓
● web                     feature  3↓  4M   5h
↑ payments-service        develop  ·       3w   2 unpushed
↓ legacy-portal           master   47↓     24w  stash
✓ platform/billing-api    main     ·        6d

86 repos   ● 1   ↑ 1   ↓ 20   ✓ 64
```

| Glyph | State                                                                            |
| ----- | -------------------------------------------------------------------------------- |
| `✗`   | error - can't read the repo, or the fetch failed                                 |
| `≠`   | conflict - merge, rebase, cherry-pick or revert in progress                      |
| `⇅`   | diverged - ahead and behind its upstream                                         |
| `●`   | dirty - staged, modified or conflicted files                                     |
| `↑`   | unpushed - commits only here: on a branch never pushed, or ahead of its upstream |
| `↓`   | behind - behind upstream, or the local default branch is behind its remote       |
| `✓`   | clean                                                                            |

A branch whose upstream was deleted on the forge (an old `develop` after a merge) is shown as a stale branch, not as unpushed work.

## Interactive View
`mrm` on a terminal opens the fleet view: every repo in one grouped, live list. It paints from the cache at once, then refreshes in the background. `mrm -w work -f "is:dirty"` starts on a workspace and filter; piped, or with `--json`, `mrm` prints `mrm status` instead.

```
 mrm  all · 78 repos    ● 2  ↑ 1  ↓ 3  ✓ 71  ✗ 1                     synced 4m ago   ctrl+k
─ ▾ services · 3 ─────────────────────────────────────────────────────────────────────────
  ✓  api                 develop    ·          2h  fix: rounding in invoices
▌ ●  web                 feature/x  2↑   3M    1d  wip: checkout layout
  ⠋  docs                main       ·          5d  syncing…
─ ▾ platform · 1 ─────────────────────────────────────────────────────────────────────────
  ↓  billing-api         master     4↓         3d  chore: bump deps
 1 marked  s sync  f fetch  A clear marks  ctrl+k more
```

| Key             | Does                                                                                   |
| --------------- | -------------------------------------------------------------------------------------- |
| `j` `k` `g` `G` | move (arrows, PgUp/PgDn too)                                                           |
| `tab`           | collapse / expand the group                                                            |
| `/`             | filter as you type - same language as `-f`                                             |
| `space` `a` `A` | mark a repo (or a whole group on its header), mark everything shown, clear marks       |
| `s` `S`         | sync the marked repos, else the group or repo under the cursor / sync everything shown |
| `f` `F`         | fetch, the same way                                                                    |
| `r`             | refresh status                                                                         |
| `o` `O`         | open on the forge / open its pull requests                                             |
| `enter`         | hand off to lazygit in the repo (`tui.open_cmd`); back to mrm when it exits            |
| `e` `t`         | editor / shell in the repo                                                             |
| `l`             | detail pane: branches, stashes, recent commits                                         |
| `!`             | needs attention: everything not clean, grouped by reason                               |
| `w`             | switch workspace                                                                       |
| `ctrl+k`        | command palette - fuzzy search over every action                                       |
| `?` `q`         | help, quit                                                                             |

A sync that leaves anything skipped or failed ends on the needs-attention view. Colours follow your terminal's palette; `NO_COLOR` (or `--no-color`) keeps bold, dim and reverse only.

## Web UI
`mrm web --open` serves the web UI on this machine only and opens it; Ctrl+C stops it. The address it prints carries a new access token each run, so only you can use it.

- **Overview**: a state bar per workspace, what needs attention (grouped by reason), work at risk - unpushed commits, stashes, long-dirty trees and stale branches - and every repo, filtered as you type.
- **Repo page**: branches with tracking, stashes, the last 20 commits, links to the repo, its pull requests and CI.
- **Activity**: every run, from the CLI, the interactive view or the page.
- **Settings**: text size (Small, Medium, Large, Extra large) and theme for this browser; `web.text_size` in the config sets the default.

Sync or fetch one repo, a workspace, the filtered set or everything; rows update live in every open tab. A sync running in a terminal shows in the top bar, and its results appear when it finishes.

## What Sync Does
Per repo, in parallel: fetch (and refresh `origin/HEAD`), then fast-forward the default branch **only if the tree is clean and the branch is strictly behind**. On another branch, the default branch is updated in place and your branch is left alone. mrm **never** stashes, resets, rebases, merges or forces - anything unsafe is skipped and listed under "Needs attention".

Exit codes: `0` everything synced or current, `2` some repos need attention, `1` mrm failed.

## Filters
`-f` takes a query; terms are ANDed, `-` negates:

```bash
mrm status -f "is:dirty"                 # states and flags: is:behind is:stash is:not-default is:stale-branches ...
mrm status -f "ws:work group:platform"
mrm sync -f "tag:terraform -is:dirty"
mrm status -f "age:>90d"                 # last commit older than 90 days
mrm status -f "branch:feature/* forge:bitbucket"
```

## Shell Completion
`mrm completion` prints a completion script for bash, zsh, fish or PowerShell. Subcommands and flags complete, and so do their values: workspaces after `-w`, tags after `-t`, filter terms after `-f` (`is:<Tab>` lists every state and flag; `ws:`, `tag:`, `group:` list yours), and repo names for `mrm open`.

```bash
# bash (needs the bash-completion package, loaded by most distros' default .bashrc)
mkdir -p ~/.local/share/bash-completion/completions
mrm completion bash > ~/.local/share/bash-completion/completions/mrm

# zsh (needs `autoload -U compinit; compinit` in ~/.zshrc)
mrm completion zsh > "${fpath[1]}/_mrm"                          # Linux
mrm completion zsh > "$(brew --prefix)/share/zsh/site-functions/_mrm"  # macOS

# fish
mrm completion fish > ~/.config/fish/completions/mrm.fish
```

Open a new shell to pick it up (bash: or `source` the file; a shell where you pressed Tab after `mrm` before installing it has already decided there is no completion), then press Tab - twice to list the options. Re-run after upgrading mrm so new commands and flags complete too. `mrm completion <shell> --help` has the details for each shell.

## Config
`~/.config/mrm/config.yaml` (XDG). Repos are discovered; the config only holds workspaces and exceptions. See the [spec](docs/01-specification.md#configuration) for every key; `mrm config init` writes a commented one.

## Docs
- [Specification](docs/01-specification.md) - what mrm does and why
- [User acceptance tests](docs/02-user-acceptance-tests.md) - the definition of done per phase
- [Roadmap](docs/03-roadmap.md) - the order it gets built in
- [Developer notes](README-dev.md)
- [Changelog](CHANGELOG.md) - what changed, per release

## License
[MIT](LICENSE)

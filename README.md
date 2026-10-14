# mrm

Mass repo manager - find every git repo under your folders, keep them all current safely and in parallel, and see the state of all of them at once.

> **Status:** phase 1 (headless CLI) in progress. The TUI (phase 2) and web UI (phase 3) are not built yet - see the [roadmap](docs/03-roadmap.md).

- [mrm](#mrm)
  - [Install](#install)
  - [Quick Start](#quick-start)
    - [Shell Completion](#shell-completion)
  - [What Sync Does](#what-sync-does)
  - [Filters](#filters)
  - [Config](#config)
  - [Docs](#docs)
  - [License](#license)

## Install
```bash
go install github.com/adegoodyer/mrm@latest
# or from a checkout
task build   # bin/mrm
```

## Quick Start
```bash
mrm config init            # commented config with a workspace per clone folder found (~/src, ~/code, ~/work, ...)
mrm doctor                 # check git, config, roots, repos, ssh
mrm status -w work         # one line per repo
mrm sync --dry-run -w work # what sync would do
mrm sync -w work           # fetch + fast-forward what is safe to
mrm open api --pr          # api's pull requests on its forge
```

With no config, the current directory is the only root.

### Shell Completion
`mrm completion` prints a completion script for bash, zsh, fish or PowerShell. Subcommands and flags complete; workspace, repo and filter values don't yet.

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

```
↓ api                     main     ·        2d  on main, default develop 12↓
● web                     feature  3↓  4M   5h
↑ payments-service        develop  ·       3w   2 unpushed
↓ legacy-portal           master   47↓     24w  stash
✓ platform/billing-api    main     ·        6d

86 repos   ● 1   ↑ 1   ↓ 20   ✓ 64
```

| Glyph | State                                                                      |
| ----- | -------------------------------------------------------------------------- |
| `✗`   | error - can't read the repo, or the fetch failed                           |
| `≠`   | conflict - merge, rebase, cherry-pick or revert in progress                |
| `⇅`   | diverged - ahead and behind its upstream                                   |
| `●`   | dirty - staged, modified or conflicted files                               |
| `↑`   | unpushed - commits on no remote                                            |
| `↓`   | behind - behind upstream, or the local default branch is behind its remote |
| `✓`   | clean                                                                      |

## What Sync Does
Per repo, in parallel: fetch (and refresh `origin/HEAD`), then fast-forward the default branch **only if the tree is clean and the branch is strictly behind**. On another branch, the default branch is updated in place and your branch is left alone. mrm **never** stashes, resets, rebases, merges or forces - anything unsafe is skipped and listed under "Needs attention".

Exit codes: `0` everything synced or current, `2` some repos need attention, `1` mrm failed.

## Filters
`-f` takes a query; terms are ANDed, `-` negates:

```bash
mrm status -f "is:dirty"                 # states and flags: is:behind is:stash is:not-default ...
mrm status -f "ws:work group:platform"
mrm sync -f "tag:terraform -is:dirty"
mrm status -f "age:>90d"                 # last commit older than 90 days
mrm status -f "branch:feature/* forge:bitbucket"
```

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
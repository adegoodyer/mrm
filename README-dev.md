# mrm - Developer Notes

- [mrm - Developer Notes](#mrm---developer-notes)
  - [Layout](#layout)
  - [Rules](#rules)
  - [Tasks](#tasks)
  - [Trying a Change](#trying-a-change)
  - [Tests](#tests)
  - [Releasing](#releasing)
  - [Renovate](#renovate)
  - [Docs Images](#docs-images)
  - [Adding a Command](#adding-a-command)

## Layout
```
main.go                 build info from ldflags -> cmd
cmd/                    cobra commands; app.go loads config + discovers + filters
internal/
  model/                Repo, Status, State, Result - shared by everything
  config/               YAML config (strict, typed, never written back), overrides, `config init` template
  paths/                XDG paths
  discover/             walk roots -> repos (stops at a repo, skips hidden/ignored, depth limit)
  git/                  the git CLI: runner (no prompts, timeouts, process-group kill), status, safe ops
  engine/               worker pool + events; status, fetch, sync rules; workspace lock; post_sync hooks; prune
  fleet/                exec: a user command in many repos, parallel with line-prefixed output or serial
  filter/               the query language (is:, ws:, tag:, branch:, host:, age:, words, -negation)
  forge/                remote URL -> web / PR / CI / branch URLs ("forge" is the code's word for a VCS host; users see "VCS host" or the site's name)
  forgeapi/             forge REST clients (GitHub, GitLab, Bitbucket, Gitea): open PRs, CI state, org listings; hosts.json cache
  drift/                config keys with no repo; org repos with no clone
  schedule/             systemd user timer units for mrm sync
  secret/               token_cmd / token_env resolution, shared by notify and forgeapi
  store/                status cache, run history
  notify/               Gotify, ntfy; secrets from token_cmd / token_env only
  browser/              open a URL (browser_cmd, wslview, xdg-open, open)
  view/                 a repo as plain text (row cells, notes, attention reason, ages) - shared by CLI and TUI
  tui/                  interactive fleet view (tview): list.go custom list primitive, actions.go engine ops + hand-off, overlays.go palette/help/workspaces/detail
  runner/               one operation the safe way: workspace lock, engine run, cache + history - used by the TUI and the web UI
  web/                  web UI: server.go (routes, host check, token, CSP), state.go (pages, actions, watcher), sse.go (live events),
                        templates/pages.html, static/ (mrm.css, mrm.js, boot.js, fonts)
  term/                 ANSI colour, padding, live progress line
  logger/               zap JSON log in the state dir
  gittest/              test fixtures: bare remote + seed + clones
docs/                   user guide, spec, UAT, roadmap
  images/               README and user guide pictures; src/ holds what makes them (see Docs Images)
```

## Rules
- **Only `internal/engine` runs git against repos** (via `internal/git`). Front ends render engine events. `internal/fleet` runs the user's own command for `exec`; hooks run from the engine.
- **Sync never changes work in progress**: no stash, reset, rebase, merge commit, force, branch delete. New sync behaviour needs a fixture test in `internal/engine/engine_test.go` proving a dirty tree is untouched. Branch deletes happen only in `prune`, after confirmation, with "merged" re-checked just before.
- **VCS host APIs are read-only and opt-in**: no token for a host, no request to it. Front ends ask in the background after an operation and paint cached answers first - never block a list on the network.
- **Commands that delete or run things ask first** through `confirm()` in `cmd/confirm.go`: `--yes` skips it, and without a terminal it refuses rather than guessing.
- **No secrets in config or logs.** Endpoints take `token_cmd` / `token_env`; never log their output.
- **The TUI is tview, never Bubble Tea.** UI state is only touched on the UI goroutine: background work (engine runs, detail reads) hands results back with `app.QueueUpdateDraw`.
- **Never call `QueueUpdate`, `QueueUpdateDraw` or `Draw` from the UI goroutine** - in key handlers or inside a queued update. They wait for the UI loop that is running them: the terminal freezes and Ctrl+C dies with it (it did, 6 s after start-up, when a footer message expired). On the UI goroutine, change state and return; tview redraws after every event and queued update.
- **CLI, TUI and web show the same text** for a repo: build it in `internal/view`, colour it in the front end.
- **TUI keys live in one table** (`internal/tui/keys.go`): the key handler, the `?` help and the command palette all read it. Add a key there and to the key table in [docs/user-guide.md](docs/user-guide.md#the-interactive-view); `TestKeysDocumented` fails until you do, and `TestKeysUnique` catches a key bound twice.
- **Web: no inline script or style.** The CSP allows only `'self'`; put CSS in `static/mrm.css`, behaviour in `static/mrm.js`, and size things with classes or SVG attributes (the state bars are SVG for this reason). Templates escape everything; keep it that way.
- **Web grids need `minmax(0, 1fr)`**, not `1fr`: a long repo name or URL otherwise widens the column past the screen (it did, at every width).
- Docs: tables are aligned with `markdown-table-prettify` (the VS Code extension's formatter).
- **Every user-visible change gets a line under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md)** ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/): Added / Changed / Fixed / Removed), in the same commit as the change. A release moves `[Unreleased]` into a dated version section.

## Tasks
```bash
task              # list
task check        # gofmt check + vet + race tests - run before committing
task build        # bin/mrm
task install      # go install: replaces the mrm on your PATH with this checkout
task test:smoke ROOT=~/work   # doctor, status, sync --dry-run in a folder of clones (fetches; changes no branches)
task run -- status -w work
task docs:images  # rebuild the README and user guide pictures
```

## Trying a Change
Run the checkout's build without replacing your installed mrm:

```bash
task build && ./bin/mrm                # your real repos and config
```

To try it against the demo fleet instead - nine made-up repos in every state (behind, diverged, dirty, unpushed, a stash, PRs and a failing CI) in a throwaway folder, with nothing outside it touched:

```bash
bash docs/images/src/demo.sh /tmp/mrm-demo
HOME=/tmp/mrm-demo/home XDG_CONFIG_HOME=/tmp/mrm-demo/xdg/config XDG_CACHE_HOME=/tmp/mrm-demo/xdg/cache \
  XDG_STATE_HOME=/tmp/mrm-demo/xdg/state ./bin/mrm
```

Sync works there too: the demo's remotes resolve to local bare repos. Run `demo.sh` again for a fresh copy.

When you're happy with it, `task install` makes it the `mrm` on your PATH (`go install` puts it in `$(go env GOPATH)/bin` unless `GOBIN` is set). The interactive view remembers its last view in `~/.local/state/mrm/tui.json`; delete that file to start fresh.

## Tests
`go test ./...` builds real repos in temp dirs (`internal/gittest`): a bare remote, a seed clone that pushes "upstream" changes, and clones put into each state (dirty, diverged, mid-merge, detached, upstream gone, untracked clash, odd paths). `gittest.Isolate()` (called from `TestMain`) points git at an empty global config with a fake identity (`Dev User <dev@example.com>`), so your own git config - signing, hooks, default branch - can't leak in.

TUI tests (`internal/tui/tui_test.go`) run the real app on tcell's simulated screen: they inject keys and poll the screen text until it shows what they expect. Read the screen through `harness.text()`, which goes via the UI goroutine - reading the simulated screen directly races with drawing.

Forge clients are tested against a fake server speaking all four APIs (`internal/forgeapi/forgeapi_test.go`). `MRM_FORGE_LIVE=<owner>/<repo> go test ./internal/forgeapi -run Live` makes read-only calls to the real GitHub API with `gh auth token`.

UAT criteria marked `[A]` in [docs/02-user-acceptance-tests.md](docs/02-user-acceptance-tests.md) name the test that covers them.

## Releasing
Releases are automatic; nothing here needs doing by hand.

- **CI** ([ci.yml](.github/workflows/ci.yml)) on every PR and push to master: `task check`, `go mod tidy -diff`, `task release:check` and `task release:snapshot` (all four archives). Tools come from [mise.toml](mise.toml) - the same Go, Task and GoReleaser locally and in CI.
- **Renovate** keeps dependencies current and merges its own PRs once CI passes - see [Renovate](#renovate).
- **Release** ([release.yml](.github/workflows/release.yml)) Mondays 08:00 UTC: if master has anything since the last tag - `[Unreleased]` notes, go.mod versions, the Go version in mise.toml - [release.py](.github/scripts/release.py) picks the version, moves the notes into a dated section (dependency and Go changes added under Changed), commits `release X.Y.Z` and tags `vX.Y.Z`; the workflow pushes both, publishes the GitHub release (linux/darwin amd64/arm64 archives, that version's CHANGELOG section as notes) and tells the Go module proxy.

The version: Added, Changed, Deprecated or Removed notes -> minor; anything else (Fixed, Security, only dependency or Go bumps) -> patch. A major (1.0.0) is never picked for you: run the Release workflow from the Actions tab with a version. The same button releases early, or does a dry run; `task release:next` shows locally what Monday would ship.

So write CHANGELOG notes as you go (see Rules) and leave `[Unreleased]` for the workflow to move. A failed run emails you and changes nothing it did not push; fix master and run it again. By hand, as a fallback: commit the dated CHANGELOG section, tag, `git push origin master vX.Y.Z`, then `GITHUB_TOKEN=$(gh auth token) task release`.

The binary's version comes from ldflags (`main.version`); `go install` builds read it from the module version instead.

## Renovate
Self-hosted [Renovate](https://docs.renovatebot.com/), not the hosted Mend app: the [renovate-operator](https://github.com/mogenius/renovate-operator) in proxmox-three runs it as the `mrm` RenovateJob (`platform/renovate/renovatejob-mrm.yaml` there), daily at 06:30 UK, as the GitHub App `proxmox-three-renovate`. What it does here is [renovate.json](renovate.json).

### What It Updates
| File                 | What                                                                                                                |
| -------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `go.mod`, `go.sum`   | module versions, then `go mod tidy`; a major also rewrites the import paths. Minor and patch updates come as one PR |
| `mise.toml`          | Go (what builds the release binaries), Task, GoReleaser                                                             |
| `.github/workflows/` | the actions, pinned to a commit SHA with the version in a comment - Renovate moves both                             |

Not go.mod's `go` line: it is the minimum Go mrm needs, and Renovate leaves it alone by default. Raise it by hand when the code needs a newer Go.

### How a PR Goes
1. An upstream release is at least 3 days old (`minimumReleaseAge`) - a bad or hijacked release usually gets pulled sooner.
2. A run opens the PR, labelled `dependencies`; CI runs on it.
3. The next run merges it (a merge commit) if CI passed, and rebases any other open PR that is now behind.
4. The Monday [release](#releasing) ships whatever was merged; the CHANGELOG line for it is written then.

A PR whose CI fails stays open - the one thing that needs you. Either fix mrm on master (Renovate then rebases the PR, CI runs again and the next run merges it), or push the fix to the PR's branch and merge it yourself - Renovate stops touching a branch someone else has pushed to.

### Watching It
- the **Dependency Dashboard** issue: every update Renovate knows about, open PRs, and a checkbox to retry or force one
- PRs labelled `dependencies`
- the operator UI, https://renovate.talos-cluster.dev (LAN only): runs, logs, **run now** for anything sooner than tomorrow 06:30
- a run's log: `kubectl -n renovate-operator get jobs`, then `kubectl -n renovate-operator logs job/<name>`

### Holding a Version Back
Add a `packageRules` entry to [renovate.json](renovate.json) with a `description` saying why and since when, as proxmox-three's does - `"allowedVersions": "!/^1\\.2\\.3$/"` skips one release, `"enabled": false` stops updates to a module. Remove it once it no longer applies. Closing a PR without merging also works: Renovate does not offer that update again (for a major, that major).

## Docs Images
The pictures in the README and the [user guide](docs/user-guide.md) are mrm's real output on a demo fleet of nine made-up repos (`docs/images/src/demo.sh`: acme remotes, `Dev User <dev@example.com>`, seeded PR and CI answers), so they show what mrm actually draws. Rebuild them after changing anything visible:

```bash
task docs:images    # Linux: needs python3 and util-linux script
```

| Image                                  | Made by                                                                                                                               |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `status.svg`, `sync.svg`               | `mrm status` / `mrm sync` run under `script` for colour, then `src/term2svg.py ansi`                                                  |
| `tui.svg`, `tui-detail.svg`            | `internal/tui/screenshot_test.go` dumps a frame of the real TUI (skipped unless `MRM_SCREENSHOT` is set), then `src/term2svg.py grid` |
| `how-it-works.svg`, `sync-rules.svg`   | `src/diagrams.py` - edit the text there when behaviour changes                                                                        |
| `web-overview-*.png`, `web-repo-*.png` | by hand, below                                                                                                                        |

The SVGs follow the reader's light or dark theme (`prefers-color-scheme`) and place every character on a grid, so they line up whatever monospace font the reader has.

Web screenshots: build a demo fleet with `bash docs/images/src/demo.sh /tmp/mrm-demo github`, then run `mrm web` against it with `HOME`, `GIT_CONFIG_NOSYSTEM=1` and the `XDG_*` variables pointing into `/tmp/mrm-demo` (as `mrm_in` in `src/make.sh` does). Take each page at 1280 px wide, 1.5x, once with the browser in light mode and once in dark: Overview 1280 x 1000, the `services/web` Repo page 1280 x 500. Save them as `docs/images/web-{overview,repo}-{light,dark}.png`, reduced to a 256-colour palette (about 40-85 KB each).

## Adding a Command
1. `cmd/<name>.go` with a `cobra.Command`, registered in `init()`.
2. Use `newApp()` for config + engine and `a.repos(ctx)` for the selected, filtered repos.
3. Return `&exitError{code: 2}` when repos need attention; plain errors exit 1.
4. Add it to the CLI table in the spec and a UAT row.
5. Add it to the commands table in the README and a section in the [user guide](docs/user-guide.md).
6. Note it under `[Unreleased]` in `CHANGELOG.md`.

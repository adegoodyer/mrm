# mrm - Developer Notes

- [mrm - Developer Notes](#mrm---developer-notes)
  - [Layout](#layout)
  - [Rules](#rules)
  - [Tasks](#tasks)
  - [Tests](#tests)
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
  engine/               worker pool + events; status, fetch, sync rules; workspace lock
  filter/               the query language (is:, ws:, tag:, branch:, forge:, age:, words, -negation)
  forge/                remote URL -> web / PR / CI / branch URLs
  store/                status cache, run history
  notify/               Gotify, ntfy; secrets from token_cmd / token_env only
  browser/              open a URL (browser_cmd, wslview, xdg-open, open)
  term/                 ANSI colour, padding, ages, live progress line
  logger/               zap JSON log in the state dir
  gittest/              test fixtures: bare remote + seed + clones
docs/                   spec, UAT, roadmap
```

## Rules
- **Only `internal/engine` runs git against repos** (via `internal/git`). Front ends render engine events.
- **Sync never changes work in progress**: no stash, reset, rebase, merge commit, force, branch delete. New sync behaviour needs a fixture test in `internal/engine/engine_test.go` proving a dirty tree is untouched.
- **No secrets in config or logs.** Endpoints take `token_cmd` / `token_env`; never log their output.
- **No Bubble Tea** for the TUI (phase 2) - see the spec's decisions.
- Docs: tables are aligned with `markdown-table-prettify` (the VS Code extension's formatter).
- **Every user-visible change gets a line under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md)** ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/): Added / Changed / Fixed / Removed), in the same commit as the change. A release moves `[Unreleased]` into a dated version section.

## Tasks
```bash
task              # list
task check        # gofmt check + vet + race tests - run before committing
task build        # bin/mrm
task test:smoke ROOT=~/work   # doctor, status, sync --dry-run in a folder of clones (fetches; changes no branches)
task run -- status -w work
```

## Tests
`go test ./...` builds real repos in temp dirs (`internal/gittest`): a bare remote, a seed clone that pushes "upstream" changes, and clones put into each state (dirty, diverged, mid-merge, detached, upstream gone, untracked clash, odd paths). `gittest.Isolate()` (called from `TestMain`) points git at an empty global config with a fake identity (`Dev User <dev@example.com>`), so your own git config - signing, hooks, default branch - can't leak in.

UAT criteria marked `[A]` in [docs/02-user-acceptance-tests.md](docs/02-user-acceptance-tests.md) name the test that covers them.

## Adding a Command
1. `cmd/<name>.go` with a `cobra.Command`, registered in `init()`.
2. Use `newApp()` for config + engine and `a.repos(ctx)` for the selected, filtered repos.
3. Return `&exitError{code: 2}` when repos need attention; plain errors exit 1.
4. Add it to the CLI table in the spec and a UAT row.
5. Note it under `[Unreleased]` in `CHANGELOG.md`.

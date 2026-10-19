package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/adegoodyer/mrm/internal/paths"
)

// Folders where people commonly keep their clones. `config init` offers a
// workspace, named after the folder, for each one that exists.
var candidateRoots = []string{"~/src", "~/code", "~/projects", "~/repos", "~/work", "~/dev", "~/git"}

var initTemplate = template.Must(template.New("config").Parse(`# mrm config - https://github.com/adegoodyer/mrm/blob/master/docs/01-specification.md#configuration
# Edit with: mrm config edit    Check with: mrm doctor

concurrency: 8            # repos worked on at once
timeout: 2m               # per repo, per git command
browser_cmd: ""           # default: wslview on WSL, else xdg-open / open

# Self-hosted forges, by host, so "open" knows their URL layout.
# forges:
#   git.example.com: gitlab

workspaces:
{{- range .Workspaces}}
  {{.Name}}:
    roots: [{{.Root}}]
    orgs: []              # host/owner to compare with, for mrm drift, e.g. [github.com/acme]
    depth: 3              # how deep to look for repos under each root
    ignore: []            # name or relative-path globs, e.g. [legacy-*]
    sync:
      remote: origin
      ff_current: false   # also fast-forward a non-default current branch
      switch_merged: false # switch to the default branch when the current one is merged
{{- end}}

# Exceptions only - repos are discovered. Keys are repo names, name globs,
# or path globs (with a / or a leading ~).
repos: {}
#   payments-service:
#     branch: develop     # instead of origin/HEAD
#     tags: [backend]
#   "~/work/terraform-*":
#     tags: [terraform]
#   legacy-portal:
#     sync: false         # status only

# Commands run after a sync moved HEAD, in repos that have if_file.
# hooks:
#   post_sync:
#     - name: pre-commit
#       if_file: .pre-commit-config.yaml
#       run: pre-commit install --install-hooks

notify:
  on: attention           # attention | always | never
#   gotify:
#     url: <gotify-url>
#     token_cmd: pass show gotify/mrm   # secrets only via token_cmd or token_env
#   ntfy:
#     url: <ntfy-url>/<topic>
#     token_env: MRM_NTFY_TOKEN

# Read-only API tokens, by host, for open PR counts, CI state and mrm drift.
# Hosts without one are never contacted.
forge_tokens: {}
#   github.com:
#     token_cmd: gh auth token
#   gitlab.com:
#     token_env: GITLAB_TOKEN
#   bitbucket.org:
#     user: <email>       # Bitbucket API tokens use Basic auth
#     token_cmd: pass show bitbucket/mrm
forge_cache: 10m          # how long forge answers are reused

tui:
  open_cmd: lazygit
  fetch_on_start: false

web:
  port: 7420
  text_size: medium       # small | medium | large | extra-large (each browser can change it)
`))

type initRoot struct{ Name, Root string }

// InitContent renders a commented default config with one workspace per
// candidate root that exists, or the current directory if none do.
func InitContent() ([]byte, error) {
	var roots []initRoot
	for _, c := range candidateRoots {
		if fi, err := os.Stat(paths.ExpandHome(c)); err == nil && fi.IsDir() {
			roots = append(roots, initRoot{filepath.Base(c), c})
		}
	}
	if len(roots) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		roots = append(roots, initRoot{filepath.Base(wd), paths.Tilde(wd)})
	}
	var buf bytes.Buffer
	if err := initTemplate.Execute(&buf, struct{ Workspaces []initRoot }{roots}); err != nil {
		return nil, err
	}
	// The template must always produce a valid config.
	if _, err := Parse(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("internal: default config is invalid: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteInit writes the default config to path. It refuses to overwrite an
// existing file unless force is set.
func WriteInit(path string, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", paths.Tilde(path))
	}
	data, err := InitContent()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

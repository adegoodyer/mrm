// Package config loads and validates mrm's YAML config. The file is only
// ever read here; mrm writes it once, on `config init`.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/adegoodyer/mrm/internal/paths"
)

const (
	DefaultConcurrency = 8
	DefaultTimeout     = 30 * time.Second
	DefaultDepth       = 3
	DefaultRemote      = "origin"
	DefaultWebPort     = 7420
)

// Config is the whole config file.
type Config struct {
	Concurrency int                   `yaml:"concurrency"`
	Timeout     Duration              `yaml:"timeout"`
	BrowserCmd  string                `yaml:"browser_cmd"`
	Forges      map[string]string     `yaml:"hosts"`       // self-hosted host -> github|gitlab|bitbucket|gitea
	ForgeTokens map[string]ForgeToken `yaml:"host_tokens"` // host -> API token, for PR/CI counts and drift
	ForgeCache  Duration              `yaml:"host_cache"`  // how long answers from the hosts' APIs are reused (default 10m)

	// The names before 0.3.0, still read; Renamed lists the ones used.
	OldForges      map[string]string     `yaml:"forges"`
	OldForgeTokens map[string]ForgeToken `yaml:"forge_tokens"`
	OldForgeCache  Duration              `yaml:"forge_cache"`
	Renamed        []string              `yaml:"-"`
	Workspaces     map[string]*Workspace `yaml:"workspaces"`
	Repos          map[string]Override   `yaml:"repos"`
	Hooks          Hooks                 `yaml:"hooks"`
	Notify         Notify                `yaml:"notify"`
	TUI            TUI                   `yaml:"tui"`
	Web            Web                   `yaml:"web"`

	// Path is where the config was loaded from; empty when there was no file.
	Path string `yaml:"-"`
}

// Workspace is a named set of roots with its own sync policy.
type Workspace struct {
	Name   string     `yaml:"-"`
	Roots  []string   `yaml:"roots"`
	Orgs   []string   `yaml:"orgs"` // host/owner on a forge, for mrm drift
	Depth  int        `yaml:"depth"`
	Ignore []string   `yaml:"ignore"`
	Sync   SyncPolicy `yaml:"sync"`
}

// SyncPolicy controls what a sync may do beyond fetch + fast-forward of the
// default branch.
type SyncPolicy struct {
	Remote       string `yaml:"remote"`
	FFCurrent    bool   `yaml:"ff_current"`
	SwitchMerged bool   `yaml:"switch_merged"`
	FFDirty      bool   `yaml:"ff_dirty"` // fast-forward over local changes the update doesn't touch
}

// Override is a per-repo exception, keyed by repo name or path glob.
type Override struct {
	Branch string   `yaml:"branch"`
	Tags   []string `yaml:"tags"`
	Sync   *bool    `yaml:"sync"`
}

// ForgeToken authenticates to one forge host. The secret only comes from a
// command or an environment variable. Bitbucket app passwords and API
// tokens need the account as user (HTTP Basic); without user it is sent as
// a Bearer token.
type ForgeToken struct {
	User     string `yaml:"user"`
	TokenCmd string `yaml:"token_cmd"`
	TokenEnv string `yaml:"token_env"`
}

// Hooks run after a sync (phase 4).
type Hooks struct {
	PostSync []Hook `yaml:"post_sync"`
}

// Hook is one post-sync command.
type Hook struct {
	Name   string `yaml:"name"`
	IfFile string `yaml:"if_file"`
	Run    string `yaml:"run"`
}

// Notify configures notifications after a headless sync.
type Notify struct {
	On     string    `yaml:"on"` // attention | always | never
	Gotify *Endpoint `yaml:"gotify"`
	Ntfy   *Endpoint `yaml:"ntfy"`
}

// Endpoint is a notification target. Secrets only come from a command or
// an environment variable.
type Endpoint struct {
	URL      string `yaml:"url"`
	TokenCmd string `yaml:"token_cmd"`
	TokenEnv string `yaml:"token_env"`
}

// TUI settings (phase 2).
type TUI struct {
	OpenCmd      string `yaml:"open_cmd"`
	FetchOnStart bool   `yaml:"fetch_on_start"`
	Mouse        *bool  `yaml:"mouse"` // default true
}

// MouseOn reports whether the interactive view takes the mouse.
func (t TUI) MouseOn() bool { return t.Mouse == nil || *t.Mouse }

// Web settings (phase 3).
type Web struct {
	Port     int    `yaml:"port"`
	TextSize string `yaml:"text_size"` // small | medium | large | extra-large; default for every browser
}

// TextSizes maps web.text_size to the root font size in px.
var TextSizes = map[string]int{"small": 14, "medium": 16, "large": 18, "extra-large": 20}

// Duration reads "2m", "90s" and so on.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a duration (e.g. 90s, 2m)", n.Line, s)
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

// Load reads the config at path. A missing file is not an error: the
// result is the no-config default (the current directory as the only root).
func Load(path string) (*Config, error) {
	if path == "" {
		path = paths.Config()
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NoConfig()
	}
	if err != nil {
		return nil, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", paths.Tilde(path), err)
	}
	cfg.Path = path
	return cfg, nil
}

// Parse decodes and validates config YAML. Unknown keys are errors.
func Parse(data []byte) (*Config, error) {
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty file decodes to io.EOF: treat it as all defaults.
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, explain(err)
	}
	if err := cfg.takeOldNames(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// takeOldNames moves keys written with their pre-0.3.0 names (forges,
// forge_tokens, forge_cache) to the new ones, and notes them for doctor.
func (c *Config) takeOldNames() error {
	type pair struct {
		old, new string
		oldSet   bool
		newSet   bool
		move     func()
	}
	for _, p := range []pair{
		{"forges", "hosts", c.OldForges != nil, c.Forges != nil, func() { c.Forges = c.OldForges }},
		{"forge_tokens", "host_tokens", c.OldForgeTokens != nil, c.ForgeTokens != nil, func() { c.ForgeTokens = c.OldForgeTokens }},
		{"forge_cache", "host_cache", c.OldForgeCache.Duration != 0, c.ForgeCache.Duration != 0, func() { c.ForgeCache = c.OldForgeCache }},
	} {
		if !p.oldSet {
			continue
		}
		if p.newSet {
			return fmt.Errorf("both %s and %s are set: %s is the old name, keep only %s", p.old, p.new, p.old, p.new)
		}
		p.move()
		c.Renamed = append(c.Renamed, p.old+" -> "+p.new)
	}
	c.OldForges, c.OldForgeTokens, c.OldForgeCache = nil, nil, Duration{}
	return nil
}

// explain adds a hint to the common mistakes.
func explain(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "field token ") || strings.Contains(msg, "field password ") {
		return fmt.Errorf("%w\nsecrets can't be written in the config: use token_cmd (e.g. `pass show gotify/mrm`) or token_env", err)
	}
	return err
}

// NoConfig is the config used when there is no file: one workspace whose
// root is the current directory.
func NoConfig() (*Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	name := filepath.Base(wd)
	if name == "" || name == string(filepath.Separator) {
		name = "default"
	}
	cfg := &Config{Workspaces: map[string]*Workspace{name: {Roots: []string{wd}}}}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	if c.Timeout.Duration <= 0 {
		c.Timeout.Duration = DefaultTimeout
	}
	if c.Notify.On == "" {
		c.Notify.On = "attention"
	}
	if c.TUI.OpenCmd == "" {
		c.TUI.OpenCmd = "lazygit"
	}
	if c.Web.Port == 0 {
		c.Web.Port = DefaultWebPort
	}
	if c.ForgeCache.Duration <= 0 {
		c.ForgeCache.Duration = 10 * time.Minute
	}
	if c.Web.TextSize == "" {
		c.Web.TextSize = "medium"
	}
	if c.Workspaces == nil {
		c.Workspaces = map[string]*Workspace{}
	}
	for name, ws := range c.Workspaces {
		if ws == nil {
			ws = &Workspace{}
			c.Workspaces[name] = ws
		}
		ws.Name = name
		if ws.Depth <= 0 {
			ws.Depth = DefaultDepth
		}
		if ws.Sync.Remote == "" {
			ws.Sync.Remote = DefaultRemote
		}
		for i, r := range ws.Roots {
			ws.Roots[i] = filepath.Clean(paths.ExpandHome(r))
		}
	}
}

// Validate checks the values the decoder can't.
func (c *Config) Validate() error {
	var errs []string
	if len(c.Workspaces) == 0 {
		errs = append(errs, "no workspaces: add at least one under `workspaces:`")
	}
	for _, ws := range c.Workspaces {
		if len(ws.Roots) == 0 {
			errs = append(errs, fmt.Sprintf("workspace %q has no roots", ws.Name))
		}
		for _, g := range ws.Ignore {
			if _, err := filepath.Match(g, ""); err != nil {
				errs = append(errs, fmt.Sprintf("workspace %q: bad ignore glob %q", ws.Name, g))
			}
		}
	}
	for key := range c.Repos {
		if _, err := filepath.Match(paths.ExpandHome(key), ""); err != nil {
			errs = append(errs, fmt.Sprintf("repos: bad glob %q", key))
		}
	}
	switch c.Notify.On {
	case "attention", "always", "never":
	default:
		errs = append(errs, fmt.Sprintf("notify.on must be attention, always or never, not %q", c.Notify.On))
	}
	for name, ep := range map[string]*Endpoint{"gotify": c.Notify.Gotify, "ntfy": c.Notify.Ntfy} {
		if ep != nil && ep.URL == "" {
			errs = append(errs, fmt.Sprintf("notify.%s.url is required", name))
		}
	}
	for host, t := range c.ForgeTokens {
		if t.TokenCmd == "" && t.TokenEnv == "" {
			errs = append(errs, fmt.Sprintf("host_tokens.%s needs token_cmd or token_env", host))
		}
	}
	for _, ws := range c.Workspaces {
		for _, o := range ws.Orgs {
			if host, owner, ok := strings.Cut(o, "/"); !ok || host == "" || owner == "" || strings.Contains(owner, "/") && !strings.Contains(host, "gitlab") {
				errs = append(errs, fmt.Sprintf("workspace %q: orgs entry %q should be host/owner, e.g. github.com/acme", ws.Name, o))
			}
		}
	}
	if _, ok := TextSizes[c.Web.TextSize]; !ok {
		errs = append(errs, fmt.Sprintf("web.text_size must be small, medium, large or extra-large, not %q", c.Web.TextSize))
	}
	for host, kind := range c.Forges {
		switch kind {
		case "github", "gitlab", "bitbucket", "gitea":
		default:
			errs = append(errs, fmt.Sprintf("hosts.%s must be github, gitlab, bitbucket or gitea, not %q", host, kind))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// WorkspaceNames returns the workspace names in sorted order.
func (c *Config) WorkspaceNames() []string {
	names := make([]string, 0, len(c.Workspaces))
	for n := range c.Workspaces {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Select returns the named workspaces in sorted order, or all of them when
// names is empty.
func (c *Config) Select(names []string) ([]*Workspace, error) {
	if len(names) == 0 {
		names = c.WorkspaceNames()
	}
	var out []*Workspace
	seen := map[string]bool{}
	for _, n := range names {
		ws, ok := c.Workspaces[n]
		if !ok {
			return nil, fmt.Errorf("no workspace %q (have: %s)", n, strings.Join(c.WorkspaceNames(), ", "))
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, ws)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Resolved is the merged override for one repo.
type Resolved struct {
	Branch string
	Tags   []string
	NoSync bool
}

// Resolve merges every override that matches a repo. Keys with a `/` or a
// leading `~` are path globs matched against the repo path; other keys are
// name globs. Path globs apply first, then name globs, then exact names, so
// the most specific key wins; tags are unioned.
func (c *Config) Resolve(name, path string) Resolved {
	type hit struct {
		rank int
		key  string
	}
	var hits []hit
	for key := range c.Repos {
		switch {
		case strings.Contains(key, "/") || strings.HasPrefix(key, "~"):
			if ok, _ := filepath.Match(paths.ExpandHome(key), path); ok {
				hits = append(hits, hit{0, key})
			}
		case key == name:
			hits = append(hits, hit{2, key})
		default:
			if ok, _ := filepath.Match(key, name); ok {
				hits = append(hits, hit{1, key})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return hits[i].key < hits[j].key
	})
	var r Resolved
	seen := map[string]bool{}
	for _, h := range hits {
		o := c.Repos[h.key]
		if o.Branch != "" {
			r.Branch = o.Branch
		}
		if o.Sync != nil {
			r.NoSync = !*o.Sync
		}
		for _, t := range o.Tags {
			if !seen[t] {
				seen[t] = true
				r.Tags = append(r.Tags, t)
			}
		}
	}
	sort.Strings(r.Tags)
	return r
}

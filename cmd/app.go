package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/filter"
	"github.com/adegoodyer/mrm/internal/logger"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/term"
	"go.uber.org/zap"
)

// app is what every command needs: config, engine and output style.
type app struct {
	cfg        *config.Config
	eng        *engine.Engine
	style      term.Style
	workspaces []*config.Workspace
}

func configPath() string {
	if flagConfig != "" {
		return paths.ExpandHome(flagConfig)
	}
	if p := os.Getenv("MRM_CONFIG"); p != "" {
		return paths.ExpandHome(p)
	}
	return paths.Config()
}

func newApp() (*app, error) {
	logger.Init(paths.Log(), flagVerbose)
	cfg, err := config.Load(configPath())
	if err != nil {
		return nil, err
	}
	wss, err := cfg.Select(flagWorkspaces)
	if err != nil {
		return nil, err
	}
	logger.L.Debug("config loaded", zap.String("path", cfg.Path), zap.Int("workspaces", len(wss)))
	return &app{
		cfg:        cfg,
		eng:        engine.New(cfg.Timeout.Duration, cfg.Concurrency),
		style:      term.NewStyle(os.Stdout, flagNoColor || flagJSON),
		workspaces: wss,
	}, nil
}

func (a *app) workspaceNames() []string {
	names := make([]string, len(a.workspaces))
	for i, ws := range a.workspaces {
		names[i] = ws.Name
	}
	return names
}

// query builds the filter from --filter and --tag.
func query() (filter.Filter, error) {
	q := flagFilter
	for _, t := range flagTags {
		q += " tag:" + t
	}
	return filter.Parse(strings.TrimSpace(q))
}

// repos discovers the selected workspaces and applies the filter. When the
// filter needs status it is read first and returned, keyed by path.
func (a *app) repos(ctx context.Context) ([]model.Repo, map[string]model.Status, error) {
	f, err := query()
	if err != nil {
		return nil, nil, err
	}
	f = f.WithForges(a.cfg.Forges)
	repos, errs := discover.All(a.cfg, a.workspaces)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, a.style.Yellow("warning: ")+e.Error())
	}
	if f.Empty() {
		return repos, nil, nil
	}
	var statuses map[string]model.Status
	if f.NeedsStatus() {
		statuses = map[string]model.Status{}
		for _, r := range a.eng.Collect(ctx, repos, engine.Options{Op: engine.OpStatus}) {
			statuses[r.Repo.Path] = r.Status
		}
	}
	var out []model.Repo
	for _, r := range repos {
		if f.Match(r, statuses[r.Path]) {
			out = append(out, r)
		}
	}
	return out, statuses, nil
}

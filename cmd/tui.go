package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/term"
	"github.com/adegoodyer/mrm/internal/tui"
)

// interactive is true when mrm can take over the terminal.
func interactive() bool {
	return !flagJSON && term.IsTTY(os.Stdin) && term.IsTTY(os.Stdout)
}

// runTUI opens the fleet view. -w with one workspace starts on it; -f and
// -t become the starting filter.
func runTUI(ctx context.Context) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	if _, err := query(); err != nil {
		return err
	}
	q := flagFilter
	for _, t := range flagTags {
		q += " tag:" + t
	}
	ws := ""
	if len(flagWorkspaces) == 1 {
		ws = flagWorkspaces[0]
	}
	if flagNoColor {
		os.Setenv("NO_COLOR", "1") // tcell drops colour, keeps bold/dim/reverse
	}
	return tui.Run(ctx, tui.Options{
		Config:    a.cfg,
		Engine:    a.eng,
		Workspace: ws,
		Query:     strings.TrimSpace(q),
		Forge:     forgeapi.New(a.cfg, paths.ForgeCache()),
		StatePath: paths.TUIState(),
	})
}

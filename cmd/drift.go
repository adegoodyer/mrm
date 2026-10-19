package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/drift"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/paths"
)

var flagDriftArchived bool

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Find config entries with no repo, and org repos not cloned",
	Long: `Compare the disk with what the config and the forges say.

Exact-name and exact-path keys under repos: that match no repo on disk are
listed (glob keys are not: matching nothing may be what they mean). For each
workspace's orgs, the org's repos are listed from the forge (a token in
forge_tokens is needed) and compared by remote URL with every clone in every
workspace; the ones not cloned are printed with a clone command. Archived
repos are counted, and listed with --archived. Repos matching the
workspace's ignore globs are left out.

mrm only reports: it never clones. Exit code 2 when there is drift.`,
	Example: `  mrm drift
  mrm drift -w work --archived
  mrm drift --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDrift(cmd.Context())
	},
}

func init() {
	driftCmd.Flags().BoolVar(&flagDriftArchived, "archived", false, "also list archived org repos that aren't cloned")
	rootCmd.AddCommand(driftCmd)
}

type driftReport struct {
	StaleKeys []drift.Stale `json:"stale_keys"`
	Orgs      []drift.Org   `json:"orgs"`
}

func runDrift(ctx context.Context) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	all, _ := a.cfg.Select(nil)
	repos, _ := discover.All(a.cfg, all)
	rep := driftReport{StaleKeys: drift.StaleKeys(a.cfg, repos), Orgs: []drift.Org{}}

	fc := forgeapi.New(a.cfg, paths.ForgeCache())
	type wsOrg struct {
		org, root string
		ignore    []string
	}
	var orgs []wsOrg
	for _, ws := range a.workspaces {
		root := ""
		if len(ws.Roots) > 0 {
			root = paths.ExpandHome(ws.Roots[0])
		}
		for _, o := range ws.Orgs {
			orgs = append(orgs, wsOrg{o, root, ws.Ignore})
		}
	}
	roots := map[string]string{}
	if len(orgs) > 0 {
		local := map[string]bool{}
		for _, r := range a.eng.Collect(ctx, repos, engine.Options{Op: engine.OpStatus}) {
			if k := drift.RemoteKey(r.Status.RemoteURL); k != "" {
				local[k] = true
			}
		}
		for _, o := range orgs {
			listed, err := fc.OrgRepos(ctx, o.org)
			if err != nil {
				rep.Orgs = append(rep.Orgs, drift.Org{Org: o.org, Err: err.Error()})
				continue
			}
			rep.Orgs = append(rep.Orgs, drift.CompareOrg(o.org, listed, local, o.ignore))
			roots[o.org] = o.root
		}
	}

	found := len(rep.StaleKeys) > 0
	for _, o := range rep.Orgs {
		found = found || len(o.NotCloned) > 0 || o.Err != ""
	}
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return err
		}
	} else {
		a.printDrift(rep, roots, len(orgs) > 0)
	}
	if found {
		return &exitError{code: 2}
	}
	return nil
}

func (a *app) printDrift(rep driftReport, roots map[string]string, haveOrgs bool) {
	s := a.style
	if len(rep.StaleKeys) > 0 {
		fmt.Println(s.Bold("Config entries with no repo on disk"))
		for _, k := range rep.StaleKeys {
			fmt.Printf("  %s  %s\n", s.Yellow("repos: "+k.Key), s.Dim(k.Why))
		}
		fmt.Println()
	}
	for _, o := range rep.Orgs {
		if o.Err != "" {
			fmt.Printf("%s  %s\n\n", s.Bold(o.Org), s.Red(o.Err))
			continue
		}
		head := fmt.Sprintf("%s  %d listed, %d not cloned", s.Bold(o.Org), o.Listed, len(o.NotCloned))
		if len(o.Archived) > 0 {
			head += fmt.Sprintf(", %d archived not cloned", len(o.Archived))
		}
		if o.Ignored > 0 {
			head += fmt.Sprintf(", %d ignored", o.Ignored)
		}
		fmt.Println(head)
		list := o.NotCloned
		if flagDriftArchived {
			list = append(list, o.Archived...)
		}
		names := make([]string, len(list))
		w := 0
		for i, r := range list {
			names[i] = path.Base(r.Path)
			if r.Archived {
				names[i] += " (archived)"
			}
			w = max(w, len(names[i]))
		}
		for i, r := range list {
			hint := ""
			if r.CloneURL != "" && roots[o.Org] != "" {
				hint = "git clone " + r.CloneURL + " " + paths.Tilde(filepath.Join(roots[o.Org], path.Base(r.Path)))
			}
			fmt.Printf("  %-*s  %s\n", w, names[i], s.Dim(hint))
		}
		fmt.Println()
	}
	switch {
	case len(rep.StaleKeys) == 0 && !haveOrgs:
		fmt.Println("No drift: every repos: entry has a repo. Add orgs: to a workspace to compare it with a forge.")
	case len(rep.StaleKeys) == 0 && !anyNotCloned(rep.Orgs):
		fmt.Println("No drift.")
	}
}

func anyNotCloned(orgs []drift.Org) bool {
	for _, o := range orgs {
		if len(o.NotCloned) > 0 || o.Err != "" {
			return true
		}
	}
	return false
}

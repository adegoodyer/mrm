package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/browser"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/filter"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/model"
)

var (
	flagOpenPR     bool
	flagOpenCI     bool
	flagOpenBranch bool
	flagOpenPrint  bool
)

var openCmd = &cobra.Command{
	Use:   "open [repo]",
	Short: "Open a repo, its pull requests, its CI or its branch in the browser",
	Long: `Open a repo in the browser on its VCS host (GitHub, GitLab, Bitbucket, Gitea/Codeberg),
working out the web URL from the remote. With no repo, the repo you are in.
A repo is matched by name, by group/name, then by part of its name or path.`,
	Example: `  mrm open api
  mrm open api --pr
  mrm open platform/billing-api --ci
  mrm open --branch            # the current repo's current branch
  mrm open api --print         # print the URL instead`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOpen(cmd.Context(), args)
	},
}

func init() {
	f := openCmd.Flags()
	f.BoolVar(&flagOpenPR, "pr", false, "pull / merge requests")
	f.BoolVar(&flagOpenCI, "ci", false, "pipelines / actions")
	f.BoolVar(&flagOpenBranch, "branch", false, "the current branch")
	f.BoolVar(&flagOpenPrint, "print", false, "print the URL, don't open it")
	openCmd.MarkFlagsMutuallyExclusive("pr", "ci", "branch")
	rootCmd.AddCommand(openCmd)
}

func runOpen(ctx context.Context, args []string) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	var repo model.Repo
	if len(args) == 0 {
		wd, _ := os.Getwd()
		top, err := a.eng.Git.TopLevel(ctx, wd)
		if err != nil {
			return fmt.Errorf("not in a git repo: name one, e.g. mrm open <repo>")
		}
		repo = model.Repo{Name: top, Path: top, Remote: "origin"}
	} else {
		repo, err = a.findRepo(args[0])
		if err != nil {
			return err
		}
	}
	st := a.eng.Git.ReadStatus(ctx, repo.Path, remoteOf(repo), repo.Branch)
	if st.Error != "" {
		return fmt.Errorf("%s: %s", repo.Display(), st.Error)
	}
	f := forge.Parse(st.RemoteURL, a.cfg.Forges)
	var url string
	switch {
	case flagOpenPR:
		url, err = f.PRs()
	case flagOpenCI:
		url, err = f.CI()
	case flagOpenBranch:
		url, err = f.Branch(st.Branch)
	default:
		url, err = f.Web()
	}
	if err != nil {
		return fmt.Errorf("%s: %w", repo.Display(), err)
	}
	if flagOpenPrint {
		fmt.Println(url)
		return nil
	}
	fmt.Fprintln(os.Stderr, "opening "+url)
	return browser.Open(a.cfg.BrowserCmd, url)
}

func remoteOf(r model.Repo) string {
	if r.Remote == "" {
		return "origin"
	}
	return r.Remote
}

// findRepo matches a name: exact name, then group/name, then part of the
// name or path. More than one match is an error listing them.
func (a *app) findRepo(name string) (model.Repo, error) {
	repos, _ := discover.All(a.cfg, a.workspaces)
	pick := func(match func(model.Repo) bool) []model.Repo {
		var out []model.Repo
		for _, r := range repos {
			if match(r) {
				out = append(out, r)
			}
		}
		return out
	}
	candidates := pick(func(r model.Repo) bool { return r.Name == name })
	if len(candidates) == 0 {
		candidates = pick(func(r model.Repo) bool { return r.Display() == name })
	}
	if len(candidates) == 0 {
		f, err := filter.Parse(name)
		if err != nil {
			return model.Repo{}, err
		}
		candidates = pick(func(r model.Repo) bool { return f.Match(r, model.Status{}) })
	}
	switch len(candidates) {
	case 0:
		return model.Repo{}, fmt.Errorf("no repo matches %q", name)
	case 1:
		return candidates[0], nil
	}
	var names []string
	for _, r := range candidates {
		names = append(names, r.Workspace+":"+r.Display())
	}
	sort.Strings(names)
	if len(names) > 10 {
		names = append(names[:10], "…")
	}
	return model.Repo{}, fmt.Errorf("%q matches %d repos: %s", name, len(candidates), strings.Join(names, ", "))
}

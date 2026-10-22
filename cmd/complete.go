package cmd

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/filter"
)

// Shell completion for values: workspaces (-w), tags (-t), filter terms
// (-f) and repo names (mrm open). Each reads the config quietly; any error
// just means no suggestions.

// registerCompletions is called from root.go's init, after the flags exist:
// init order follows file names, and this file sorts before root.go.
func registerCompletions() {
	for flag, fn := range map[string]cobra.CompletionFunc{
		"workspace": completeWorkspaces,
		"tag":       completeTags,
		"filter":    completeFilter,
	} {
		if err := rootCmd.RegisterFlagCompletionFunc(flag, fn); err != nil {
			panic("completion for --" + flag + ": " + err.Error())
		}
	}
	openCmd.ValidArgsFunction = completeRepos
}

const noFiles = cobra.ShellCompDirectiveNoFileComp

func completionConfig() *config.Config {
	cfg, err := config.Load(configPath())
	if err != nil {
		return nil
	}
	return cfg
}

func completeWorkspaces(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := completionConfig()
	if cfg == nil {
		return nil, noFiles
	}
	return withPrefix(cfg.WorkspaceNames(), toComplete), noFiles
}

func completeTags(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := completionConfig()
	if cfg == nil {
		return nil, noFiles
	}
	return withPrefix(configTags(cfg), toComplete), noFiles
}

// completeRepos offers repo names and group/name for `mrm open`.
func completeRepos(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	cfg := completionConfig()
	if cfg == nil {
		return nil, noFiles
	}
	wss, err := cfg.Select(flagWorkspaces)
	if err != nil {
		return nil, noFiles
	}
	repos, _ := discover.All(cfg, wss)
	seen := map[string]bool{}
	var out []string
	for _, r := range repos {
		for _, name := range []string{r.Name, r.Display()} {
			if !seen[name] && strings.HasPrefix(name, toComplete) {
				seen[name] = true
				out = append(out, name+"\t"+r.Workspace)
			}
		}
	}
	sort.Strings(out)
	return out, noFiles
}

// completeFilter completes the last term of a -f query: a key ("is:",
// "ws:", ...) or, after a key, its values. A leading "-" (negation) is kept.
func completeFilter(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	head, last := "", toComplete
	if i := strings.LastIndex(toComplete, " "); i >= 0 {
		head, last = toComplete[:i+1], toComplete[i+1:]
	}
	neg := ""
	if strings.HasPrefix(last, "-") {
		neg, last = "-", last[1:]
	}
	key, value, hasKey := strings.Cut(last, ":")
	if !hasKey {
		var keys []string
		for _, k := range filter.Keys() {
			keys = append(keys, k+":")
		}
		return prefixed(head+neg, withPrefix(keys, last)), noFiles | cobra.ShellCompDirectiveNoSpace
	}
	var values []string
	switch key {
	case "is":
		values = filter.IsValues()
	case "host":
		values = []string{"github", "gitlab", "bitbucket", "gitea", "unknown"}
	case "age":
		values = []string{">1d", ">7d", ">30d", ">90d", ">1y", "<1d", "<7d"}
	case "ws", "tag", "group":
		cfg := completionConfig()
		if cfg == nil {
			return nil, noFiles
		}
		switch key {
		case "ws":
			values = cfg.WorkspaceNames()
		case "tag":
			values = configTags(cfg)
		case "group":
			values = groups(cfg)
		}
	}
	return prefixed(head+neg+key+":", withPrefix(values, value)), noFiles
}

func configTags(cfg *config.Config) []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range cfg.Repos {
		for _, t := range o.Tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

func groups(cfg *config.Config) []string {
	wss, err := cfg.Select(flagWorkspaces)
	if err != nil {
		return nil
	}
	repos, _ := discover.All(cfg, wss)
	seen := map[string]bool{}
	var out []string
	for _, r := range repos {
		if r.Group != "" && !seen[r.Group] {
			seen[r.Group] = true
			out = append(out, r.Group)
		}
	}
	sort.Strings(out)
	return out
}

func withPrefix(items []string, prefix string) []string {
	var out []string
	for _, s := range items {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func prefixed(p string, items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = p + s
	}
	return out
}

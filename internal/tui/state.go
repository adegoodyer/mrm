package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// viewState is what the interactive view remembers between runs.
type viewState struct {
	Workspace string   `json:"workspace"`
	Collapsed []string `json:"collapsed,omitempty"`
	Detail    bool     `json:"detail,omitempty"`
}

// loadState restores the last view. -w on the command line wins over the
// saved workspace, and a workspace no longer in the config is ignored.
func (u *ui) loadState() {
	if u.opts.StatePath == "" {
		return
	}
	data, err := os.ReadFile(u.opts.StatePath)
	if err != nil {
		return
	}
	var st viewState
	if json.Unmarshal(data, &st) != nil {
		return
	}
	if u.opts.Workspace == "" {
		if _, ok := u.cfg.Workspaces[st.Workspace]; ok || st.Workspace == "" {
			u.ws = st.Workspace
		}
	}
	for _, k := range st.Collapsed {
		u.collapsed[k] = true
	}
	if st.Detail && !u.showDetail {
		u.toggleDetail()
	}
}

// saveState writes the view for next time. Failing to is not worth an error.
func (u *ui) saveState() {
	if u.opts.StatePath == "" {
		return
	}
	st := viewState{Workspace: u.ws, Detail: u.showDetail}
	for k, c := range u.collapsed {
		if c {
			st.Collapsed = append(st.Collapsed, k)
		}
	}
	sort.Strings(st.Collapsed)
	data, _ := json.MarshalIndent(st, "", "  ")
	if os.MkdirAll(filepath.Dir(u.opts.StatePath), 0o755) == nil {
		_ = os.WriteFile(u.opts.StatePath, data, 0o644)
	}
}

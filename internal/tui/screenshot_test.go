package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forgeapi"
)

// TestScreenshot dumps one frame of the real TUI as a cell grid for the
// docs images (docs/images/src). It runs only when asked:
//
//	MRM_SCREENSHOT=<out.json> MRM_CONFIG=<config> MRM_SCREENSHOT_CACHE=<dir>
//	MRM_SCREENSHOT_KEYS="jj l" MRM_SCREENSHOT_SIZE=100x22 go test ./internal/tui -run Screenshot
//
// The cache dir holds the status.json and forge.json to paint from (TestMain
// points XDG at an empty temp dir). MRM_SCREENSHOT_HOME shortens paths to ~,
// and MRM_SCREENSHOT_HISTORY gives the "synced 4m ago" in the header.
func TestScreenshot(t *testing.T) {
	out := os.Getenv("MRM_SCREENSHOT")
	if out == "" {
		t.Skip("set MRM_SCREENSHOT to write a frame")
	}
	cache := os.Getenv("MRM_SCREENSHOT_CACHE")
	if home := os.Getenv("MRM_SCREENSHOT_HOME"); home != "" {
		t.Setenv("HOME", home)
	}
	hist := os.Getenv("MRM_SCREENSHOT_HISTORY")
	if hist == "" {
		hist = filepath.Join(t.TempDir(), "runs.jsonl")
	}
	cfg, err := config.Load(os.Getenv("MRM_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	w, h := 100, 22
	if s := os.Getenv("MRM_SCREENSHOT_SIZE"); s != "" {
		for i := range s {
			if s[i] == 'x' {
				w, _ = strconv.Atoi(s[:i])
				h, _ = strconv.Atoi(s[i+1:])
			}
		}
	}
	scr := tcell.NewSimulationScreen("UTF-8")
	u, err := newUI(context.Background(), Options{
		Config: cfg, Engine: engine.New(30*time.Second, 4), Screen: scr, NoRefresh: true,
		CachePath: filepath.Join(cache, "status.json"), HistPath: hist,
		Forge: forgeapi.New(cfg, filepath.Join(cache, "forge.json")),
	})
	if err != nil {
		t.Fatal(err)
	}
	scr.SetSize(w, h)
	done := make(chan error, 1)
	go func() { done <- u.run() }()
	defer func() { u.app.Stop(); <-done }()
	time.Sleep(300 * time.Millisecond)
	for _, r := range os.Getenv("MRM_SCREENSHOT_KEYS") {
		if r == '_' { // _ stands for space (mark)
			r = ' '
		}
		scr.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		time.Sleep(60 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond) // the detail pane reads git in the background

	type cell struct {
		R    string `json:"r"`
		FG   int    `json:"fg"`
		Bold bool   `json:"bold,omitempty"`
		Dim  bool   `json:"dim,omitempty"`
		Rev  bool   `json:"rev,omitempty"`
	}
	grid := struct {
		Cols int      `json:"cols"`
		Rows [][]cell `json:"rows"`
	}{Cols: w}
	ch := make(chan struct{})
	u.app.QueueUpdate(func() {
		cells, cw, chh := scr.GetContents()
		for y := 0; y < chh; y++ {
			var row []cell
			for x := 0; x < cw; x++ {
				c := cells[y*cw+x]
				fg, _, attr := c.Style.Decompose()
				idx := -1
				if fg&tcell.ColorIsRGB == 0 && fg >= tcell.ColorValid {
					idx = int(fg - tcell.ColorValid)
				}
				r := " "
				if len(c.Runes) > 0 && c.Runes[0] != 0 {
					r = string(c.Runes)
				}
				row = append(row, cell{R: r, FG: idx, Bold: attr&tcell.AttrBold != 0, Dim: attr&tcell.AttrDim != 0, Rev: attr&tcell.AttrReverse != 0})
			}
			grid.Rows = append(grid.Rows, row)
		}
		close(ch)
	})
	<-ch
	data, _ := json.Marshal(grid)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

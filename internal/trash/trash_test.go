package trash

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestMove(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("freedesktop trash layout")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	xdg.Reload()
	src := t.TempDir()
	for range 2 {
		dir := filepath.Join(src, "api")
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Move(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("still there")
		}
	}
	for _, name := range []string{"api", "api 2"} {
		if _, err := os.Stat(filepath.Join(Dir(), "files", name, ".git")); err != nil {
			t.Fatalf("%s not in the trash: %v", name, err)
		}
		info, err := os.ReadFile(filepath.Join(Dir(), "info", name+".trashinfo"))
		if err != nil || !strings.Contains(string(info), "Path="+filepath.Join(src, "api")) {
			t.Fatalf("%s info: %q %v", name, info, err)
		}
	}
}

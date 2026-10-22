// Package trash moves folders to the user's trash instead of deleting
// them, so they can be restored: the freedesktop.org trash on Linux (and
// WSL), ~/.Trash on macOS.
package trash

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/adrg/xdg"
)

// Dir is where trashed folders go.
func Dir() string {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".Trash")
	}
	return filepath.Join(xdg.DataHome, "Trash")
}

// Move puts path in the trash and returns where it went.
func Move(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		dest := free(Dir(), filepath.Base(path), "")
		if err := os.Rename(path, dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	files, info := filepath.Join(Dir(), "files"), filepath.Join(Dir(), "info")
	if err := os.MkdirAll(files, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(info, 0o700); err != nil {
		return "", err
	}
	dest := free(files, filepath.Base(path), info)
	name := filepath.Base(dest)
	// The .trashinfo file is what lets a file manager restore it.
	infoPath := filepath.Join(info, name+".trashinfo")
	body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		(&url.URL{Path: path}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(infoPath, []byte(body), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(path, dest); err != nil {
		os.Remove(infoPath)
		var le *os.LinkError
		if errors.As(err, &le) && errors.Is(le.Err, syscall.EXDEV) {
			// Another filesystem (a Windows drive on WSL, a second disk):
			// gio knows that drive's own trash.
			if gio, lerr := exec.LookPath("gio"); lerr == nil {
				if out, gerr := exec.Command(gio, "trash", path).CombinedOutput(); gerr != nil {
					return "", fmt.Errorf("gio trash: %s", strings.TrimSpace(string(out)))
				}
				return "the trash of the drive it was on", nil
			}
			return "", fmt.Errorf("%s is on another drive than the trash, and gio isn't installed to use that drive's trash", path)
		}
		return "", err
	}
	return dest, nil
}

// free is a name in dir that isn't taken: base, then "base 2", "base 3"...
func free(dir, base, infoDir string) string {
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s %d", base, i)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err != nil {
			if infoDir == "" {
				return p
			}
			if _, err := os.Lstat(filepath.Join(infoDir, name+".trashinfo")); err != nil {
				return p
			}
		}
	}
}

package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// ErrLocked means another sync holds a workspace lock.
var ErrLocked = errors.New("sync already running")

// Lock holds one workspace lock per name; Release frees them all.
type Lock struct{ files []*os.File }

// LockWorkspaces takes a non-blocking lock for each workspace, in sorted
// order so two runs can't deadlock. The lock file holds the holder's pid.
func LockWorkspaces(dir string, names []string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	l := &Lock{}
	for _, n := range sorted {
		path := filepath.Join(dir, n+".lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			l.Release()
			return nil, err
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			pid := readPID(f)
			f.Close()
			l.Release()
			if pid != "" {
				return nil, fmt.Errorf("%w for workspace %s (pid %s)", ErrLocked, n, pid)
			}
			return nil, fmt.Errorf("%w for workspace %s", ErrLocked, n)
		}
		_ = f.Truncate(0)
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
		l.files = append(l.files, f)
	}
	return l, nil
}

// Release frees every lock held.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	for _, f := range l.files {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
	l.files = nil
}

func readPID(f *os.File) string {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	return strings.TrimSpace(string(buf[:n]))
}

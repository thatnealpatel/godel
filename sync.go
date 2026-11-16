package godel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

func claimRun() {
	if !runCalled.CompareAndSwap(false, true) {
		panic("godel: Run may be called only once per process lifetime")
	}
}

var runCalled atomic.Bool

func acquireProgramLock(p *program) (*os.File, error) {
	path := filepath.Join(p.Dir, "godel.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("godel: open lock: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		err = errors.Join(err, f.Close())
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			panic("godel: program " + p.ID + " already running (godel.lock is held)")
		}
		return nil, fmt.Errorf("godel: lock: %w", err)
	}
	return f, nil
}

// lockManifest uses a separate inode so
// replacing manifest.json cannot bypass
// another process's lock. The caller closes
// the file to release the lock.
func lockManifest() (string, *os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("find storage home: %w", err)
	}
	root := filepath.Join(home, ".godel")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", nil, fmt.Errorf("create storage %q: %w", root, err)
	}
	lock, err := os.OpenFile(filepath.Join(root, "manifest.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("open manifest lock: %w", err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return "", nil, fmt.Errorf("lock manifest: %w", errors.Join(err, lock.Close()))
	}
	return root, lock, nil
}

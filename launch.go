package godel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// launchFresh replaces this runner with go run in the live source.
// The Go tool rebuilds that source and starts main again with a
// fresh run identity. The unlinked file provides finite stdin
// without a pipe-writing goroutine that would disappear at exec.
// Stdout and stderr are inherited unchanged.
func launchFresh(ctx context.Context, source, input string) error {
	path, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("godel: find go for fresh launch: %w", err)
	}
	stdin, err := os.CreateTemp("", "godel-stdin-*")
	if err != nil {
		return fmt.Errorf("godel: create fresh stdin: %w", err)
	}
	defer stdin.Close()
	if err := os.Remove(stdin.Name()); err != nil {
		return fmt.Errorf("godel: unlink fresh stdin: %w", err)
	}
	if _, err := stdin.WriteString(input); err != nil {
		return fmt.Errorf("godel: write fresh stdin: %w", err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		return fmt.Errorf("godel: rewind fresh stdin: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("godel: prepare fresh launch: %w", err)
	}
	if err := os.Chdir(source); err != nil {
		return fmt.Errorf("godel: enter fresh source %q: %w", source, err)
	}
	fd := stdin.Fd()
	if fd == 0 {
		// A closed stdin lets CreateTemp reuse fd 0.
		// Dup2 may use dup3, which rejects identical
		// descriptors; instead make this descriptor
		// survive exec, preserving other flags.
		flags, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
		if err != nil {
			return fmt.Errorf("godel: get fresh stdin descriptor flags: %w", err)
		}
		if _, err := unix.FcntlInt(fd, unix.F_SETFD, flags&^unix.FD_CLOEXEC); err != nil {
			return fmt.Errorf("godel: clear fresh stdin close-on-exec: %w", err)
		}
	} else if err := unix.Dup2(int(fd), 0); err != nil {
		return fmt.Errorf("godel: duplicate fresh stdin onto fd 0: %w", err)
	}
	args := append([]string{"go", "run", "."}, os.Args[1:]...)
	err = syscall.Exec(path, args, os.Environ())
	// Keep the File owning fd 0 alive
	// through exec, not just its integer
	// descriptor. The deferred Close also
	// retains it.
	runtime.KeepAlive(stdin)
	return fmt.Errorf("godel: launch fresh go run: %w", err)
}

package godel

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// nodeCommand requires a user systemd manager. The scope, rather
// than a process group, owns descendants even if they change
// process groups.
func nodeCommand(ctx context.Context, n *Node) (*exec.Cmd, error) {
	// Resolve using the runner's PATH, as
	// exec.Command did before wrapping the
	// executable. Node Env overrides must
	// not change executable lookup.
	path, err := exec.LookPath(n.Exec)
	if err != nil {
		return nil, err
	}
	unit := "godel-exec-" + rand.Text()
	args := []string{"--user", "--scope", "--quiet", "--collect", "--unit=" + unit}
	// Scope mode inherits cwd and stdio.
	// Apply node overrides only to the
	// payload, so overriding its bus address
	// cannot break scope management.
	for _, entry := range n.Env {
		args = append(args, "--setenv="+entry)
	}
	args = append(args, "--", path)
	args = append(args, n.Args...)
	cmd := exec.CommandContext(ctx, "systemd-run", args...)
	cmd.Env = systemdEnv()
	cmd.Cancel = func() error { return cancelScope(cmd, unit+".scope") }
	return cmd, nil
}

func systemdEnv() []string {
	runtimeDir := filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	return append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(runtimeDir, "bus"))
}

// cancelScope uses an independent context because
// the node's is already cancelled. During startup,
// even a successful kill can precede attachment to
// the new scope. Keep killing until the launcher
// exits; tolerate only the unit-not-loaded race,
// within the same overall bound.
func cancelScope(cmd *exec.Cmd, unit string) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		kill := exec.CommandContext(ctx, "systemctl", "--user", "kill", "-s", "SIGKILL", unit)
		kill.Env = append(systemdEnv(), "LC_ALL=C")
		output, err := kill.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "Unit "+unit+" not loaded.") {
			return fmt.Errorf("kill user systemd scope %s: %w; stderr: %s", unit, err, strings.TrimSpace(string(output)))
		}
		if errors.Is(cmd.Process.Signal(syscall.Signal(0)), os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("kill user systemd scope %s: %w: %v; stderr: %s", unit, ctx.Err(), err, strings.TrimSpace(string(output)))
		case <-timer.C:
		}
	}
}

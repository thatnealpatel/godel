package godel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInvokeCancellationKillsScope(t *testing.T) {
	requireSystemd(t)
	// Isolate subreaping from the rest of the suite.
	// The test must reap killed grandchildren, not
	// leave zombies to the host's init process.
	if os.Getenv("GODEL_CANCELLATION_TEST") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestInvokeCancellationKillsScope$")
		cmd.Env = append(os.Environ(), "GODEL_CANCELLATION_TEST=1")
		cmd.WaitDelay = 2 * time.Second
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cancellation regression: %v\n%s", err, output)
		}
		return
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	pidPath := filepath.Join(directory, "root.pid")
	ready := filepath.Join(directory, "ready")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	node := &Node{
		Exec: executable,
		Args: []string{"-test.run=^TestCancellationProcessHelper$", "--", "root", pidPath, ready},
		Env:  []string{"GODEL_CANCELLATION_HELPER=1"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	type observation struct {
		metadata metadata
		err      error
	}
	resultCh := make(chan observation, 1)
	finished := false
	t.Cleanup(func() {
		cancel()
		// Clean up known PIDs even if readiness
		// or cancellation assertions fail.
		// Production cancellation must kill the
		// entire scope instead.
		data, err := os.ReadFile(pidPath)
		if err == nil {
			pid, err := strconv.Atoi(string(data))
			if err != nil || pid <= 0 {
				t.Fatalf("invalid root PID %q: %v", data, err)
			}
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Errorf("cleanup root: %v", err)
			}
		}
		if data, err := os.ReadFile(ready); err == nil {
			var childPID, groupPID int
			if _, err := fmt.Sscanf(string(data), "%d %d", &childPID, &groupPID); err == nil && childPID > 0 {
				if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
					t.Errorf("cleanup child: %v", err)
				}
			}
		}
		if !finished {
			select {
			case <-resultCh:
			case <-time.After(5 * time.Second):
				t.Error("invoke did not finish during cleanup")
			}
		}
		// Only this isolated test's adopted
		// grandchildren remain after invoke
		// has waited for the root.
		waitForCancellationExit(t, -1)
	})
	go func() {
		_, metadata, err := invoke(ctx, node, "cancel me")
		resultCh <- observation{
			metadata: metadata,
			err:      err,
		}
	}()

	data := waitForCancellationFile(t, ready)
	var childPID, groupPID int
	if _, err := fmt.Sscanf(string(data), "%d %d", &childPID, &groupPID); err != nil {
		t.Fatal(err)
	}
	rootData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	rootPID, err := strconv.Atoi(string(rootData))
	if err != nil {
		t.Fatal(err)
	}
	if childPID <= 0 || groupPID != childPID || groupPID == rootPID || groupPID == syscall.Getpgrp() {
		t.Fatalf("child PID/group = %d/%d, want a separate group from root %d", childPID, groupPID, rootPID)
	}
	cancel()
	select {
	case got := <-resultCh:
		finished = true
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("invoke error = %v, want context cancellation", got.err)
		}
		m := got.metadata
		if m.Error == "" || m.Error == context.Canceled.Error() {
			t.Fatalf("lost original command error: %+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("invoke did not return after cancellation")
	}

	// A descendant that changed process
	// groups must still be killed.
	waitForCancellationExit(t, childPID)
}

func waitForCancellationExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status syscall.WaitStatus
		got, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
		if errors.Is(err, syscall.ECHILD) && pid == -1 {
			return
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got > 0 {
			if !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Errorf("child %d status = %v, want SIGKILL", got, status)
			}
			if pid != -1 {
				return
			}
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for child %d to terminate", pid)
}

func waitForCancellationFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(strings.Fields(string(data))) == 2 {
			return data
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return nil
}

// TestCancellationProcessHelper runs in
// subprocesses of the cancellation test.
func TestCancellationProcessHelper(t *testing.T) {
	if os.Getenv("GODEL_CANCELLATION_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(90)
	}
	args = args[1:]
	if args[0] == "child" && len(args) == 2 {
		group, err := syscall.Getpgid(0)
		if err != nil {
			os.Exit(91)
		}
		data := fmt.Sprintf("%d %d", os.Getpid(), group)
		if err := os.WriteFile(args[1], []byte(data), 0600); err != nil {
			os.Exit(92)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if args[0] != "root" || len(args) != 3 {
		os.Exit(93)
	}
	if err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(94)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestCancellationProcessHelper$", "--", "child", args[2])
	// Leave the root's process group while retaining its
	// output pipes. Process-group cancellation would leave
	// this child alive.
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(95)
	}
	// Do not reap here: the isolated test
	// adopts and checks the child's status.
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func TestCancelScopeNoProcess(t *testing.T) {
	if err := cancelScope(&exec.Cmd{}, "godel-missing.scope"); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("cancel without process = %v, want os.ErrProcessDone", err)
	}
}

func TestCancelScopeAlreadyExited(t *testing.T) {
	requireSystemd(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := cancelScope(cmd, "godel-missing.scope"); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("cancel exited scope = %v, want os.ErrProcessDone", err)
	}
}

// requireSystemd keeps process tests
// usable on hosts without a user
// manager. It uses the real executable
// and manager, never a simulated scope.
func requireSystemd(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemd-run", "--user", "--scope", "--quiet", "--collect", "--", "/bin/true")
	cmd.Env = systemdEnv()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("working user systemd manager unavailable: %v: %s", err, output)
	}
}

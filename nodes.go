package godel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// executeTree checks nodes only as they are executed. All continuations receive
// the original input, never a preceding process's output. A Stop continuation
// preserves the current result.
func executeTree(ctx context.Context, root *Node, input string) (execution, error) {
	return executeTreeWith(ctx, root, input, invoke)
}

func executeTreeWith(ctx context.Context, root *Node, input string, run func(context.Context, *Node, string) (Result, metadata, error)) (execution, error) {
	var out execution
	for n := root; ; {
		if n == nil {
			return out, errors.New("godel: nil node in execution tree")
		}
		if n.stop {
			out.Stopped = true
			return out, nil
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if n.Exec == "" {
			return out, errors.New("godel: ordinary node requires an executable")
		}
		result, metadata, err := run(ctx, n, input)
		out.Result = result
		out.Invocations = append(out.Invocations, metadata)
		if result.Session != "" {
			out.Sessions = append(out.Sessions, result.Session)
		}
		if err != nil {
			return out, err
		}
		if n.Next == nil {
			return out, nil
		}
		next, err := n.Next(result)
		if err != nil {
			out.Invocations[len(out.Invocations)-1].Error = err.Error()
			return out, fmt.Errorf("godel: next after %q (session %q): %w", n.Exec, result.Session, err)
		}
		if next == nil {
			return out, nil
		}
		n = next
	}
}

type execution struct {
	Result      Result
	Stopped     bool
	Sessions    []string
	Invocations []metadata
}

// invoke runs an ordinary work node with diagnostics sent directly to the
// runner's stderr. Improvement uses invokeWithStderr to prefix its auxiliary
// diagnostics: otherwise a composing runner could mistake an improvement's
// yah-session marker for the work session.
func invoke(ctx context.Context, n *Node, input string) (Result, metadata, error) {
	return invokeWithStderr(ctx, n, input, os.Stderr)
}

type metadata struct {
	Elapsed time.Duration `json:"elapsed_ns"`
	Error   string        `json:"error,omitempty"`
}

// invokeWithStderr executes directly without a shell, preserves stdout exactly,
// and tees stderr to diagnostics while retaining the raw bytes for session
// extraction and errors. Thus prefixing displayed improvement diagnostics does
// not hide that attempt's session.
func invokeWithStderr(ctx context.Context, n *Node, input string, diagnostics io.Writer) (out Result, m metadata, retErr error) {
	if n == nil {
		return out, m, errors.New("godel: cannot invoke nil node")
	}
	started := time.Now()
	defer func() {
		m.Elapsed = time.Since(started)
	}()
	cmd, err := nodeCommand(ctx, n)
	if err != nil {
		m.Error = err.Error()
		return out, m, fmt.Errorf("godel: execute %q: %w", n.Exec, err)
	}
	// A failed/cancelled harness may leave
	// descendants holding its output pipes.
	// Bound waiting for those pipes instead
	// of hanging Run forever.
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = n.Directory
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(diagnostics, &stderr)
	err = cmd.Run()
	if err != nil {
		m.Error = err.Error()
	}
	out = Result{
		Text:    stdout.String(),
		Session: sessionID(stderr.String()),
	}
	if err == nil {
		return out, m, nil
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out, m, fmt.Errorf("godel: execute %q (session %q): %w; stderr: %s", n.Exec, out.Session, err, stderr.String())
}

func sessionID(stderr string) string {
	var session string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSuffix(line, "\r")
		id, ok := strings.CutPrefix(line, "yah-session: ")
		if ok && id != "" && !strings.ContainsAny(id, " \t\r\v\f") {
			session = id
		}
	}
	return session
}

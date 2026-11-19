package godel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInvokeProcessContract(t *testing.T) {
	t.Setenv("GODEL_TEST_INHERITED", "parent")
	t.Setenv("GODEL_TEST_OVERRIDE", "parent")
	t.Setenv("TOOLS", "default-tools")
	n := helperNode(t, "inspect", "literal spaces", "$(not-a-shell)", "", "a'b\"c")
	n.Directory = t.TempDir()
	n.Env = append(n.Env, "GODEL_TEST_OVERRIDE=first", "GODEL_TEST_OVERRIDE=child", "GODEL_TEST_EMPTY=", "DBUS_SESSION_BUS_ADDRESS=node-bus", "XDG_RUNTIME_DIR=node-runtime", "PATH=node-path")
	input := " task\n\n\x00with trailing space \n"
	before := time.Now()
	result, m, err := invoke(context.Background(), n, input)
	after := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args  []string
		Env   []string
		Cwd   string
		Input string
	}
	if err := json.Unmarshal([]byte(result.Text), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args, []string{"literal spaces", "$(not-a-shell)", "", "a'b\"c"}) || got.Input != input || got.Cwd != n.Directory {
		t.Fatalf("process contract mismatch: %+v", got)
	}
	env := make(map[string]string)
	for _, entry := range got.Env {
		key, value, _ := strings.Cut(entry, "=")
		if _, exists := env[key]; exists {
			t.Errorf("duplicate environment key: %s", key)
		}
		env[key] = value
	}
	for key, want := range map[string]string{"GODEL_TEST_INHERITED": "parent", "GODEL_TEST_OVERRIDE": "child", "GODEL_TEST_EMPTY": "", "TOOLS": "default-tools", "DBUS_SESSION_BUS_ADDRESS": "node-bus", "XDG_RUNTIME_DIR": "node-runtime", "PATH": "node-path"} {
		if value, exists := env[key]; !exists || value != want {
			t.Errorf("env[%q] = %q (present %v), want %q", key, value, exists, want)
		}
	}
	if m.Elapsed <= 0 || m.Elapsed > after.Sub(before) {
		t.Fatalf("invalid invocation timing: %+v; interval %v to %v", m, before, after)
	}
	if m.Error != "" {
		t.Fatalf("unexpected completion observation = %+v", m)
	}
}

func helperNode(t *testing.T, mode string, args ...string) *Node {
	t.Helper()
	requireSystemd(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &Node{
		Exec: executable,
		Args: append([]string{"-test.run=^TestNodeHelperProcess$", "--", mode}, args...),
		Env:  []string{"GODEL_TEST_HELPER=1"},
	}
}

// TestNodeHelperProcess runs only in
// subprocesses created by helperNode.
// Reading all stdin also verifies that
// invoke provides EOF, rather than an
// open pipe.
func TestNodeHelperProcess(t *testing.T) {
	if os.Getenv("GODEL_TEST_HELPER") != "1" {
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
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(91)
	}
	if want := os.Getenv("GODEL_TEST_CWD"); want != "" {
		cwd, err := os.Getwd()
		if err != nil || cwd != want {
			fmt.Fprintf(os.Stderr, "cwd = %q, %v; want %q\n", cwd, err, want)
			os.Exit(95)
		}
	}
	switch args[0] {
	case "inspect":
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(92)
		}
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			Args  []string
			Env   []string
			Cwd   string
			Input string
		}{
			args[1:],
			os.Environ(),
			cwd,
			string(input),
		})
	case "echo":
		fmt.Fprint(os.Stdout, string(input))
		fmt.Fprintln(os.Stderr, "leaf diagnostic")
	case "respond":
		if string(input) != os.Getenv("GODEL_TEST_INPUT") {
			fmt.Fprintln(os.Stderr, "altered or duplicated original input")
			os.Exit(93)
		}
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "altered response arguments")
			os.Exit(94)
		}
		fmt.Fprint(os.Stdout, args[1])
		fmt.Fprintln(os.Stderr, args[2])
	case "output":
		fmt.Fprint(os.Stdout, args[1])
		fmt.Fprint(os.Stderr, args[2])
	case "fail":
		fmt.Fprint(os.Stdout, "partial output\n")
		fmt.Fprintln(os.Stderr, "diagnostic detail\nyah-session: failed-session")
		os.Exit(23)
	case "wait":
		time.Sleep(30 * time.Second)
	default:
		os.Exit(99)
	}
	os.Exit(0)
}

func TestInvokeInheritsDirectory(t *testing.T) {
	n := helperNode(t, "inspect")
	result, _, err := invoke(context.Background(), n, "")
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Cwd string }
	if err := json.Unmarshal([]byte(result.Text), &got); err != nil {
		t.Fatal(err)
	}
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got.Cwd != want {
		t.Fatalf("cwd = %q, want %q", got.Cwd, want)
	}
}

func TestInvokePreservesOutputAndDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, stderr string
	}{
		{"multiline", "diagnostic\nyah-session: opaque-id\nother\n"},
		{"unterminated", "diagnostic without a newline"},
		{"crlf", "diagnostic\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := " \nexact output \n\n"
			capture, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			previous := os.Stderr
			os.Stderr = capture
			defer func() { os.Stderr = previous }()
			result, _, err := invoke(context.Background(), helperNode(t, "output", text, tc.stderr), "")
			if err != nil {
				t.Fatal(err)
			}
			if result.Text != text {
				t.Fatalf("result = %+v, want text %q", result, text)
			}
			tee, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			if string(tee) != tc.stderr {
				t.Fatalf("stderr tee = %q, want %q", tee, tc.stderr)
			}
		})
	}
}

func TestExecuteTreeLeaf(t *testing.T) {
	text := " unchanged leaf output \n\n"
	got, err := executeTree(context.Background(), helperNode(t, "output", text, ""), "task")
	if err != nil {
		t.Fatal(err)
	}
	want := execution{Result: Result{Text: text}}
	if !sameExecutionOutcome(got, want) {
		t.Fatalf("execution = %+v, want %+v", got, want)
	}
}

func TestExecuteTreeNestedOriginalInput(t *testing.T) {
	input := "  original task\nwith quotes \" and trailing spaces  \n"
	leaf := helperNode(t, "echo")
	inner := helperNode(t, "respond", "not JSON: continue", "inner")
	inner.Env = append(inner.Env, "GODEL_TEST_INPUT="+input)
	inner.Next = func(result Result) (*Node, error) {
		if result != (Result{Text: "not JSON: continue"}) {
			t.Fatalf("inner callback result = %+v", result)
		}
		return leaf, nil
	}
	root := helperNode(t, "respond", "arbitrary output\n", "outer")
	root.Env = append(root.Env, "GODEL_TEST_INPUT="+input)
	root.Next = func(result Result) (*Node, error) {
		if result != (Result{Text: "arbitrary output\n"}) {
			t.Fatalf("root callback result = %+v", result)
		}
		return inner, nil
	}
	got, err := executeTree(t.Context(), root, input)
	if err != nil {
		t.Fatal(err)
	}
	want := execution{Result: Result{Text: input}}
	if !sameExecutionOutcome(got, want) || len(got.Invocations) != 3 {
		t.Fatalf("execution = %+v, want %+v and three invocations", got, want)
	}
}

// sameExecutionOutcome compares tree outcomes without
// the private trace.
func sameExecutionOutcome(got, want execution) bool {
	got.Invocations = nil
	return reflect.DeepEqual(got, want)
}

func TestExecuteTreeStopAndNilLeaf(t *testing.T) {
	root := helperNode(t, "output", " unchanged response \n", "")
	root.Next = func(Result) (*Node, error) { return nil, nil }
	got, err := executeTree(t.Context(), root, "task")
	want := execution{Result: Result{Text: " unchanged response \n"}}
	if err != nil || !sameExecutionOutcome(got, want) || len(got.Invocations) != 1 {
		t.Fatalf("nil leaf = %+v, %v; want %+v and one invocation", got, err, want)
	}
	root.Next = func(Result) (*Node, error) { return Stop(), nil }
	got, err = executeTree(t.Context(), root, "task")
	want.Stopped = true
	if err != nil || !sameExecutionOutcome(got, want) || len(got.Invocations) != 1 {
		t.Fatalf("returned Stop = %+v, %v; want %+v and one invocation", got, err, want)
	}
}

func TestExecuteTreeStopRoot(t *testing.T) {
	root := Stop()
	root.Exec = "/not/executed"
	root.Next = func(Result) (*Node, error) {
		t.Fatal("Stop root invoked Next")
		return nil, nil
	}
	got, err := executeTree(t.Context(), root, "ignored")
	if err != nil || !reflect.DeepEqual(got, execution{Stopped: true}) {
		t.Fatalf("direct Stop = %+v, %v", got, err)
	}
}

func TestExecuteTreeCallbackError(t *testing.T) {
	failure := errors.New("caller could not interpret response")
	root := helperNode(t, "output", "uninterpreted output\n", "")
	root.Next = func(result Result) (*Node, error) {
		// Result is supplied by value; changes
		// cannot rewrite history.
		result.Text = "not the result"
		return Stop(), failure
	}
	got, err := executeTree(t.Context(), root, "task")
	if !errors.Is(err, failure) {
		t.Fatalf("callback error = %v, want %v", err, failure)
	}
	want := execution{Result: Result{Text: "uninterpreted output\n"}}
	if !sameExecutionOutcome(got, want) || len(got.Invocations) != 1 || got.Invocations[0].Error != failure.Error() {
		t.Fatalf("callback failure lost result or observation: %+v", got)
	}
}

func TestExecuteTreeInvalidExecutedNode(t *testing.T) {
	got, err := executeTree(t.Context(), nil, "task")
	if err == nil || !strings.Contains(err.Error(), "nil node in execution tree") || !reflect.DeepEqual(got, execution{}) {
		t.Fatalf("nil root = %+v, %v", got, err)
	}
	got, err = executeTree(t.Context(), &Node{}, "task")
	if err == nil || !strings.Contains(err.Error(), "ordinary node requires an executable") || !reflect.DeepEqual(got, execution{}) {
		t.Fatalf("invalid root = %+v, %v", got, err)
	}
	root := helperNode(t, "output", "selected", "")
	root.Next = func(Result) (*Node, error) { return &Node{}, nil }
	got, err = executeTree(t.Context(), root, "task")
	if err == nil || !strings.Contains(err.Error(), "ordinary node requires an executable") {
		t.Fatalf("invalid continuation error = %v", err)
	}
	if got.Result.Text != "selected" || len(got.Invocations) != 1 {
		t.Fatalf("invalid continuation lost prior invocation: %+v", got)
	}
}

func TestExecuteTreeSelectedCycleCancellation(t *testing.T) {
	root := helperNode(t, "output", "reply", "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	root.Next = func(Result) (*Node, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return root, nil
	}
	got, err := executeTree(ctx, root, "task")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("selected cycle error = %v, want cancellation", err)
	}
	if len(got.Invocations) != 2 || calls != 2 {
		t.Fatalf("cycle made %d calls to Next and recorded %d invocations, want two of each", calls, len(got.Invocations))
	}
}

func TestExecuteTreeFailure(t *testing.T) {
	root := helperNode(t, "fail")
	root.Next = func(Result) (*Node, error) {
		t.Fatal("Next called after failed process")
		return Stop(), nil
	}
	got, err := executeTree(t.Context(), root, "task")
	if err == nil {
		t.Fatal("expected failure")
	}
	if want := "exit status 23; stderr: diagnostic detail\nyah-session: failed-session\n"; !strings.HasSuffix(err.Error(), want) {
		t.Errorf("error = %q, want suffix %q", err, want)
	}
	want := execution{Result: Result{Text: "partial output\n"}}
	if !sameExecutionOutcome(got, want) || len(got.Invocations) != 1 || got.Invocations[0].Error != "exit status 23" {
		t.Fatalf("failed execution = %+v, want %+v and failure observation", got, want)
	}
}

func TestInvokeStartFailure(t *testing.T) {
	n := &Node{Exec: filepath.Join(t.TempDir(), "missing")}
	got, err := executeTree(t.Context(), n, "task")
	if !errors.Is(err, os.ErrNotExist) || len(got.Invocations) != 1 {
		t.Fatalf("missing executable: %+v, %v", got, err)
	}
	m := got.Invocations[0]
	if !strings.Contains(m.Error, "no such file") {
		t.Fatalf("lookup failure observation: %+v", m)
	}
	if _, _, err := invoke(context.Background(), nil, ""); err == nil {
		t.Fatal("nil invocation succeeded")
	}
}

func TestInvokeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, m, err := invoke(ctx, helperNode(t, "wait"), "task")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled invocation: %v", err)
	}
	if m.Error != context.Canceled.Error() {
		t.Fatalf("pre-start cancellation observation: %+v", m)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := executeTree(ctx, helperNode(t, "wait"), "task"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline invocation: %v", err)
	}
}

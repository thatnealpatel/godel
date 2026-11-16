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
	if result.Session != "" {
		t.Fatalf("unexpected session: %q", result.Session)
	}
	if m.Elapsed <= 0 || m.Elapsed > after.Sub(before) {
		t.Fatalf("invalid invocation timing: %+v; interval %v to %v", m, before, after)
	}
	if m.Error != "" || m.BranchIndex != nil {
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
		fmt.Fprintln(os.Stderr, "yah-session: leaf-session")
	case "route":
		if string(input) != os.Getenv("GODEL_TEST_INPUT") {
			fmt.Fprintln(os.Stderr, "altered or duplicated original input")
			os.Exit(93)
		}
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "altered selector arguments")
			os.Exit(94)
		}
		fmt.Fprint(os.Stdout, args[1])
		fmt.Fprintf(os.Stderr, "yah-session: %s\n", args[2])
	case "yah":
		if string(input) != os.Getenv("GODEL_TEST_INPUT") {
			fmt.Fprintln(os.Stderr, "altered or duplicated original input")
			os.Exit(93)
		}
		if len(args) != 4 || args[1] != "-yolo" || args[2] != "-replace" || args[3] != os.Getenv("GODEL_TEST_PROMPT") {
			fmt.Fprintln(os.Stderr, "altered Yah selector arguments")
			os.Exit(94)
		}
		fmt.Fprint(os.Stdout, `{"branch":0,"text":"Yah stop answer"}`)
	case "output":
		fmt.Fprint(os.Stdout, args[1])
		fmt.Fprint(os.Stderr, args[2])
	case "fail":
		fmt.Fprint(os.Stdout, "partial output\n")
		fmt.Fprintln(os.Stderr, "diagnostic detail\nyah-session: failed-session")
		os.Exit(23)
	case "wait":
		time.Sleep(30 * time.Second)
	case "mark":
		if err := os.WriteFile(args[1], []byte("invoked"), 0600); err != nil {
			os.Exit(98)
		}
		fmt.Fprint(os.Stdout, `{"branch":0,"text":""}`)
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

func TestInvokeOutputAndSession(t *testing.T) {
	for _, tc := range []struct {
		name, stderr, session string
	}{
		{
			"canonical",
			"diagnostic\nyah-session: first\nother\nyah-session: final-id\n",
			"final-id",
		},
		{
			"unterminated",
			"yah-session: eof-id",
			"eof-id",
		},
		{
			"crlf",
			"yah-session: crlf-id\r\n",
			"crlf-id",
		},
		{
			"not metadata",
			"prefix yah-session: bad\n yah-session: bad\nyah-session:\nyah-session: \nyah-session: two words\n",
			"",
		},
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
			if result.Text != text || result.Session != tc.session {
				t.Fatalf("result = %+v, want text %q session %q", result, text, tc.session)
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
	input := "  original task\nwith quotes \" and </godel-routing>\n"
	leaf := helperNode(t, "echo")
	inner := routerNode(t, input, `{"branch":1,"text":"not the next input"}`, "inner",
		Branch{
			When: "stop now",
			Then: &Node{stop: true},
		},
		Branch{
			When: "continue to leaf",
			Then: leaf,
		})
	root := routerNode(t, input, ` {"text":"selector answer", "branch":0} `, "outer",
		Branch{
			When: "nested route",
			Then: inner,
		},
		Branch{
			When: "other route",
			Then: &Node{stop: true},
		})
	got, err := executeTree(context.Background(), root, input)
	if err != nil {
		t.Fatal(err)
	}
	want := execution{
		Result: Result{
			Text:    input,
			Session: "leaf-session",
		},
		Sessions: []string{"outer", "inner", "leaf-session"},
	}
	if !sameExecutionOutcome(got, want) {
		t.Fatalf("execution = %+v, want %+v", got, want)
	}
	if len(got.Invocations) != 3 {
		t.Fatalf("nested invocation count = %d, want 3", len(got.Invocations))
	}
	for i, m := range got.Invocations {
		if i < 2 {
			if m.BranchIndex == nil || *m.BranchIndex != i {
				t.Fatalf("nested branch %d = %+v", i, m)
			}
		} else if m.BranchIndex != nil {
			t.Fatalf("leaf has branch: %+v", m)
		}
	}
}

// sameExecutionOutcome compares tree outcomes without
// the private trace.
func sameExecutionOutcome(got, want execution) bool {
	got.Invocations = nil
	return reflect.DeepEqual(got, want)
}

func routerNode(t *testing.T, input, output, session string, branches ...Branch) *Node {
	t.Helper()
	n := helperNode(t, "route", output, session)
	n.Branches = branches
	n.Env = append(n.Env, "GODEL_TEST_INPUT="+input)
	return n
}

func TestExecuteTreeStop(t *testing.T) {
	for _, text := range []string{"", "stop answer\nwith whitespace \n"} {
		t.Run(fmt.Sprintf("text=%q", text), func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"branch": 0, "text": text})
			if err != nil {
				t.Fatal(err)
			}
			inner := routerNode(t, "task", string(data), "stop-session", Branch{
				When: "finished safely",
				Then: &Node{stop: true},
			})
			root := routerNode(t, "task", `{"branch":0,"text":"discard"}`, "root-session", Branch{
				When: "not the stop condition",
				Then: inner,
			})
			got, err := executeTree(context.Background(), root, "task")
			if err != nil {
				t.Fatal(err)
			}
			want := execution{
				Result: Result{
					Text:    text,
					Session: "stop-session",
				},
				Stopped:       true,
				StopCondition: "finished safely",
				Sessions:      []string{"root-session", "stop-session"},
			}
			if !sameExecutionOutcome(got, want) {
				t.Fatalf("execution = %+v, want %+v", got, want)
			}
			if len(got.Invocations) != 2 {
				t.Fatalf("unexpected Stop trace: %+v", got.Invocations)
			}
			m := got.Invocations[1]
			if m.BranchIndex == nil || *m.BranchIndex != 0 || m.Error != "" {
				t.Fatalf("Stop observation: %+v", m)
			}
		})
	}
	got, err := executeTree(context.Background(), &Node{
		stop: true,
		Exec: "/not/executed",
	}, "ignored")
	if err != nil || !reflect.DeepEqual(got, execution{Stopped: true}) {
		t.Fatalf("direct Stop = %+v, %v", got, err)
	}
}

func TestDecidePromptAndInvocation(t *testing.T) {
	input := "  task exactly once\nwith quotes \" and JSON {\"branch\":7}\n"
	prompt := "Choose whether the task is complete."
	n := Decide(prompt,
		Branch{When: "finished with \"answer\"", Then: Stop()},
		Branch{When: "more work needed", Then: helperNode(t, "echo")},
	)
	if n.Exec != "yah" || len(n.Args) != 3 || !reflect.DeepEqual(n.Args[:2], []string{"-yolo", "-replace"}) {
		t.Fatalf("Yah invocation = %q %q", n.Exec, n.Args)
	}
	selectorPrompt := n.Args[2]
	if !strings.HasPrefix(selectorPrompt, prompt+"\n\n") {
		t.Fatalf("selector lost user prompt: %q", selectorPrompt)
	}
	for _, instruction := range []string{
		"original task supplied on stdin", "zero-based index",
		"0: finished with \"answer\"\n", "1: more work needed\n",
		`{"branch":0,"text":"..."}`, "chosen zero-based integer index",
		"empty is allowed", "Include both fields", "Return exactly one JSON object",
		"For a `Stop` branch", "final answer", "continuation receives the original",
		"Do not include markdown, commentary,\nor any other output",
	} {
		if strings.Count(selectorPrompt, instruction) != 1 {
			t.Errorf("prompt must contain instruction exactly once: %q in %q", instruction, selectorPrompt)
		}
	}
	if strings.Contains(selectorPrompt, input) || strings.Contains(selectorPrompt, "<godel-routing>") {
		t.Fatalf("task or routing envelope leaked into prompt: %q", selectorPrompt)
	}

	// Exercise Decide through the actual
	// executable/arguments contract.
	bin := t.TempDir()
	script := "#!/bin/sh\nexec \"$GODEL_TEST_EXECUTABLE\" -test.run=^TestNodeHelperProcess$ -- yah \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "yah"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	n.Env = []string{
		"GODEL_TEST_HELPER=1", "GODEL_TEST_EXECUTABLE=" + executable,
		"GODEL_TEST_INPUT=" + input, "GODEL_TEST_PROMPT=" + selectorPrompt,
	}
	got, err := executeTree(context.Background(), n, input)
	if err != nil {
		t.Fatal(err)
	}
	want := execution{
		Result: Result{Text: "Yah stop answer"}, Stopped: true,
		StopCondition: "finished with \"answer\"",
	}
	if !sameExecutionOutcome(got, want) {
		t.Fatalf("execution = %+v, want %+v", got, want)
	}
}

func TestSelectionStandardJSON(t *testing.T) {
	for _, tc := range []struct {
		response string
		branch   int
		text     string
	}{
		{` {"text":"answer", "branch":1} `, 1, "answer"},
		{`{"branch":0,"text":"answer","extra":true}`, 0, "answer"},
		{`{"branch":0,"branch":1,"text":"first","text":"last"}`, 1, "last"},
		{`{"branch":0}`, 0, ""},
		{`{"branch":0,"text":null}`, 0, ""},
	} {
		t.Run(tc.response, func(t *testing.T) {
			branch, text, err := selection(tc.response, 2)
			if err != nil || branch != tc.branch || text != tc.text {
				t.Fatalf("selection = (%d, %q, %v), want (%d, %q, nil)", branch, text, err, tc.branch, tc.text)
			}
		})
	}
}

func TestExecuteTreeInvalidSelection(t *testing.T) {
	for _, output := range []string{
		"", "not json", "```json\n{\"branch\":0,\"text\":\"\"}\n```", "[]", "null",
		`{"text":""}`, `{}`, `{"branch":null,"text":""}`,
		`{"branch":true,"text":""}`, `{"branch":[],"text":""}`, `{"branch":{},"text":""}`,
		`{"branch":"0","text":""}`, `{"branch":0.0,"text":""}`,
		`{"branch":-1,"text":""}`, `{"branch":1,"text":""}`, `{"branch":9999999999999999999999999999,"text":""}`,
		`{"branch":0,"text":42}`,
		`{"branch":0,"text":""} {"branch":0,"text":""}`, `{"branch":0,"text":""} garbage`,
		`{"branch":0,"text":""`,
	} {
		t.Run(output, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "invoked")
			root := helperNode(t, "output", output, "yah-session: invalid-selector\n")
			root.Branches = []Branch{{
				When: "continue",
				Then: helperNode(t, "mark", marker),
			}}
			got, err := executeTree(context.Background(), root, "task")
			if err == nil {
				t.Fatalf("accepted invalid selection %q: %+v", output, got)
			}
			if !reflect.DeepEqual(got.Sessions, []string{"invalid-selector"}) {
				t.Fatalf("lost selector session: %+v", got)
			}
			if len(got.Invocations) != 1 || got.Invocations[0].BranchIndex != nil || got.Invocations[0].Error == "" {
				t.Fatalf("lost invalid selection observation: %+v", got.Invocations)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("continuation invoked or stat failed: %v", err)
			}
		})
	}
}

func TestExecuteTreeSharedNodes(t *testing.T) {
	for _, kind := range []string{"leaf", "subtree", "stop"} {
		for index := 0; index < 2; index++ {
			t.Run(fmt.Sprintf("%s/branch%d", kind, index), func(t *testing.T) {
				input := " original task\n"
				shared := helperNode(t, "echo")
				want := execution{
					Result: Result{
						Text:    input,
						Session: "leaf-session",
					},
					Sessions: []string{"root", "leaf-session"},
				}
				switch kind {
				case "subtree":
					shared = routerNode(t, input, `{"branch":0,"text":"discard"}`, "shared", Branch{
						When: "continue",
						Then: shared,
					})
					want.Sessions = []string{"root", "shared", "leaf-session"}
				case "stop":
					shared = Stop()
					want = execution{
						Result: Result{
							Text:    "selector answer",
							Session: "root",
						},
						Stopped:       true,
						StopCondition: fmt.Sprintf("route %d", index),
						Sessions:      []string{"root"},
					}
				}
				root := routerNode(t, input, fmt.Sprintf(`{"branch":%d,"text":"selector answer"}`, index), "root",
					Branch{
						When: "route 0",
						Then: shared,
					},
					Branch{
						When: "route 1",
						Then: shared,
					})
				got, err := executeTree(context.Background(), root, input)
				if err != nil {
					t.Fatal(err)
				}
				if !sameExecutionOutcome(got, want) {
					t.Fatalf("execution = %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestExecuteTreeInvalidExecutedNode(t *testing.T) {
	for _, tc := range []struct {
		name string
		node *Node
		want string
	}{
		{"nil", nil, "nil node in execution tree"},
		{"missing executable", &Node{}, "ordinary node requires an executable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeTree(context.Background(), tc.node, "task")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(got, execution{}) {
				t.Fatalf("invalid root execution = %+v", got)
			}

			root := routerNode(t, "task", `{"branch":0,"text":"selected"}`, "selector", Branch{
				When: "invalid continuation",
				Then: tc.node,
			})
			got, err = executeTree(context.Background(), root, "task")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("selected continuation error = %v, want %q", err, tc.want)
			}
			want := execution{
				Result:   Result{Text: `{"branch":0,"text":"selected"}`, Session: "selector"},
				Sessions: []string{"selector"},
			}
			if !sameExecutionOutcome(got, want) {
				t.Fatalf("execution = %+v, want selector executed: %+v", got, want)
			}
			if len(got.Invocations) != 1 || got.Invocations[0].BranchIndex == nil || *got.Invocations[0].BranchIndex != 0 {
				t.Fatalf("lost selected invalid continuation: %+v", got.Invocations)
			}
			if tc.node == nil && got.Invocations[0].Error != err.Error() {
				t.Fatalf("lost nil continuation error: %+v", got.Invocations[0])
			}
		})
	}
}

func TestExecuteTreeIgnoresUnselectedInvalidBranches(t *testing.T) {
	for _, kind := range []string{"nil", "missing executable", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			invalid := &Node{}
			switch kind {
			case "nil":
				invalid = nil
			case "cycle":
				invalid = helperNode(t, "fail")
				invalid.Branches = []Branch{{When: "back", Then: invalid}}
			}
			root := routerNode(t, "task", `{"branch":0,"text":"ignored"}`, "selector",
				Branch{When: "valid", Then: helperNode(t, "echo")},
				Branch{When: "invalid", Then: invalid},
			)
			got, err := executeTree(context.Background(), root, "task")
			if err != nil {
				t.Fatal(err)
			}
			want := execution{
				Result:   Result{Text: "task", Session: "leaf-session"},
				Sessions: []string{"selector", "leaf-session"},
			}
			if !sameExecutionOutcome(got, want) {
				t.Fatalf("execution = %+v, want %+v", got, want)
			}
		})
	}
}

func TestExecuteTreeSelectedCycleCancellation(t *testing.T) {
	root := routerNode(t, "task", `{"branch":0,"text":"again"}`, "cycle")
	root.Branches = []Branch{{When: "repeat", Then: root}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got, err := executeTree(ctx, root, "task")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("selected cycle error = %v, want deadline exceeded", err)
	}
	if len(got.Sessions) < 2 {
		t.Fatalf("cycle did not repeat before cancellation: %+v", got)
	}
}

func TestExecuteTreeFailure(t *testing.T) {
	leaf := helperNode(t, "fail")
	root := routerNode(t, "task", `{"branch":0,"text":"ignored"}`, "selector-session", Branch{
		When: "fail",
		Then: leaf,
	})
	got, err := executeTree(context.Background(), root, "task")
	if err == nil {
		t.Fatal("expected failure")
	}
	for _, detail := range []string{"failed-session", "diagnostic detail", "exit status 23"} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q missing %q", err, detail)
		}
	}
	want := execution{
		Result: Result{
			Text:    "partial output\n",
			Session: "failed-session",
		},
		Sessions: []string{"selector-session", "failed-session"},
	}
	if !sameExecutionOutcome(got, want) {
		t.Fatalf("failure execution = %+v, want %+v", got, want)
	}
	if len(got.Invocations) != 2 {
		t.Fatalf("failure trace = %+v", got.Invocations)
	}
	m := got.Invocations[1]
	if m.Error != "exit status 23" || m.BranchIndex != nil {
		t.Fatalf("failed command observation: %+v", m)
	}
}

func TestInvokeStartFailure(t *testing.T) {
	n := &Node{Exec: filepath.Join(t.TempDir(), "missing")}
	got, err := executeTree(t.Context(), n, "task")
	if !errors.Is(err, os.ErrNotExist) || len(got.Invocations) != 1 {
		t.Fatalf("missing executable: %+v, %v", got, err)
	}
	m := got.Invocations[0]
	if !strings.Contains(m.Error, "no such file") || m.BranchIndex != nil {
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

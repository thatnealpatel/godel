package godel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImproveInvocation(t *testing.T) {
	for _, directory := range []string{"default", "explicit"} {
		t.Run(directory, func(t *testing.T) {
			n := helperNode(t, "inspect", "--print", "literal spaces", "", "$(not-a-shell)")
			n.Env = append(n.Env, "GODEL_IMPROVEMENT_ENV=supplied")
			if directory == "explicit" {
				n.Directory = t.TempDir()
			}
			before := *n
			before.Args = append([]string(nil), n.Args...)
			before.Env = append([]string(nil), n.Env...)
			info := improvementContext{
				Goal:             "Improve understanding\nwithout substituting a metric.",
				ProgramID:        "aBcDeFg",
				Source:           t.TempDir(),
				ProgramDirectory: filepath.Join(t.TempDir(), "program"),
				RunRecord:        filepath.Join(t.TempDir(), "run.json"),
				Directory:        t.TempDir(),
				Input:            "original task with \"quotes\"",
				Output:           "partial work result",
				Failure:          "original work failure",
				Loop:             true,
				Stopped:          true,
				Invocations:      []metadata{{Error: "exit status 23"}},
			}
			result, err := improve(t.Context(), n, info)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*n, before) {
				t.Fatalf("caller node changed: %+v, want %+v", *n, before)
			}
			if len(result.Invocations) != 1 {
				t.Fatalf("improvement invocations = %+v", result.Invocations)
			}
			m := result.Invocations[0]
			if m.Error != "" || m.Elapsed <= 0 {
				t.Fatalf("improvement metadata = %+v", m)
			}
			var got struct {
				Args  []string
				Env   []string
				Cwd   string
				Input string
			}
			if err := json.Unmarshal([]byte(result.Result.Text), &got); err != nil {
				t.Fatal(err)
			}
			wantDirectory := info.Source
			if n.Directory != "" {
				wantDirectory = n.Directory
			}
			if got.Cwd != wantDirectory {
				t.Errorf("invocation directory = %q, want %q", got.Cwd, wantDirectory)
			}
			if !reflect.DeepEqual(got.Args, []string{"--print", "literal spaces", "", "$(not-a-shell)"}) {
				t.Errorf("caller args changed: %q", got.Args)
			}
			if !strings.Contains("\n"+strings.Join(got.Env, "\n")+"\n", "\nGODEL_IMPROVEMENT_ENV=supplied\n") {
				t.Errorf("missing supplied environment: %q", got.Env)
			}
			prompt, context, ok := strings.Cut(got.Input, "\n\n# Run context\n\n")
			if !ok {
				t.Fatal("stdin lacks instructions followed by run context")
			}
			if strings.TrimSpace(prompt) == "" {
				t.Error("stdin lacks an improvement task")
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(context), &fields); err != nil {
				t.Fatal(err)
			}
			for key := range fields {
				if strings.EqualFold(key, "sessions") {
					t.Errorf("unexpected session collection %q in run context", key)
				}
			}
			var task improvementContext
			if err := json.Unmarshal([]byte(context), &task); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(task, info) {
				t.Errorf("task context = %+v, want %+v", task, info)
			}
		})
	}
}

func TestImproveInvocationFailure(t *testing.T) {
	leaf := helperNode(t, "fail")
	n := helperNode(t, "output", "attempt repair", "transition diagnostic")
	n.Next = func(Result) (*Node, error) { return leaf, nil }
	capture, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	previous := os.Stderr
	os.Stderr = capture
	defer func() { os.Stderr = previous }()
	result, err := improve(t.Context(), n, improvementContext{Source: t.TempDir()})
	if err == nil {
		t.Fatal("improve accepted a failed invocation")
	}
	if len(result.Invocations) != 2 {
		t.Fatalf("failed improvement invocations = %+v", result.Invocations)
	}
	if m := result.Invocations[0]; m.Error != "" {
		t.Fatalf("improvement transition metadata = %+v", m)
	}
	m := result.Invocations[1]
	if m.Error != "exit status 23" {
		t.Fatalf("failed improvement metadata = %+v", m)
	}
	if result.Result != (Result{Text: "partial output\n"}) {
		t.Fatalf("failed invocation lost stdout: %+v", result)
	}
	for _, detail := range []string{"improvement attempt", "diagnostic detail\nyah-session: failed-session"} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q missing %q", err, detail)
		}
	}
	diagnostics, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	if want := "godel: improvement: transition diagnosticgodel: improvement: diagnostic detail\ngodel: improvement: yah-session: failed-session\n"; string(diagnostics) != want {
		t.Fatalf("improvement diagnostics = %q, want %q", diagnostics, want)
	}
}

func TestImproveTreeOriginalInput(t *testing.T) {
	info := improvementContext{
		Goal:   "Improve the program, not the task",
		Source: t.TempDir(), Directory: t.TempDir(),
		Input: "original work task\n", Output: "work answer",
		Loop: true,
	}
	contextJSON, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	input := improvementPrompt + "\n\n# Run context\n\n" + string(contextJSON)
	leaf := helperNode(t, "echo")
	leaf.Directory = t.TempDir()
	leaf.Env = append(leaf.Env, "GODEL_TEST_CWD="+leaf.Directory)
	root := helperNode(t, "respond", "not the leaf input", "root")
	root.Env = append(root.Env, "GODEL_TEST_INPUT="+input)
	root.Next = func(Result) (*Node, error) { return leaf, nil }
	root.Env = append(root.Env, "GODEL_TEST_CWD="+info.Source)
	beforeRoot, beforeLeaf := *root, *leaf
	beforeRoot.Args = append([]string(nil), root.Args...)
	beforeRoot.Env = append([]string(nil), root.Env...)
	beforeRoot.Next = nil
	beforeLeaf.Args = append([]string(nil), leaf.Args...)
	beforeLeaf.Env = append([]string(nil), leaf.Env...)
	capture, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	previous := os.Stderr
	os.Stderr = capture
	defer func() { os.Stderr = previous }()

	got, err := improve(t.Context(), root, info)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != (Result{Text: input}) || got.Stopped {
		t.Fatalf("improvement result = %+v", got)
	}
	if len(got.Invocations) != 2 {
		t.Fatalf("improvement invocations = %+v", got.Invocations)
	}
	for _, m := range got.Invocations {
		if m.Error != "" {
			t.Fatalf("improvement observation = %+v", m)
		}
	}
	afterRoot := *root
	afterRoot.Next = nil
	if root.Next == nil || !reflect.DeepEqual(afterRoot, beforeRoot) || !reflect.DeepEqual(*leaf, beforeLeaf) {
		t.Fatal("improvement mutated caller nodes")
	}
	diagnostics, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "godel: improvement: root\ngodel: improvement: leaf diagnostic\n"
	if string(diagnostics) != want {
		t.Fatalf("improvement diagnostics = %q, want %q", diagnostics, want)
	}
}

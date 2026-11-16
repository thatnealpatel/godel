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
				Version:          "AbCdEfG",
				RunRecord:        filepath.Join(t.TempDir(), "run.json"),
				Directory:        t.TempDir(),
				Input:            "original task with \"quotes\"",
				Output:           "partial work result",
				Failure:          "original work failure",
				StopCondition:    "enough evidence",
				Loop:             true,
				Stopped:          true,
				Sessions:         []string{"work-session", "prior-session"},
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
			if m.Error != "" || m.BranchIndex != nil || m.Elapsed <= 0 {
				t.Fatalf("improvement metadata = %+v", m)
			}
			if result.Result.Session != "" {
				t.Fatalf("invented session for generic process: %q", result.Result.Session)
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
			if prompt != improvementPrompt || !strings.HasSuffix(prompt, improvementRider) {
				t.Error("stdin does not include the complete improvement instructions and rider")
			}
			for _, required := range []string{
				"Its `Source` field is the registered live source and your direct editing target",
				"The process operating directory may differ; it does not change the target.",
				"Use absolute paths under `Source`, or explicitly `cd` there",
				"do not use campaign workspaces to stage or activate these edits.",
				"Edit it directly.",
				"there is no version-selection",
				"Do not modify retained archives or run records.",
				"work's output as stdin, never your\n  improvement response",
				"Run records are history only",
				"A nonempty `Failure` field is the original work error",
				"`Output` may contain partial stdout from failed work",
				"without retrying or launching another run, even when `Loop` is true",
			} {
				if !strings.Contains(prompt, required) {
					t.Errorf("improvement instructions missing %q", required)
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
	n := helperNode(t, "output", `{"branch":0,"text":"discard"}`, "yah-session: selector-session")
	n.Branches = []Branch{{When: "attempt repair", Then: leaf}}
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
	selector := result.Invocations[0]
	if selector.BranchIndex == nil || *selector.BranchIndex != 0 || selector.Error != "" {
		t.Fatalf("failed improvement selector metadata = %+v", selector)
	}
	m := result.Invocations[1]
	if m.Error != "exit status 23" {
		t.Fatalf("failed improvement metadata = %+v", m)
	}
	if result.Result != (Result{Text: "partial output\n", Session: "failed-session"}) {
		t.Fatalf("failed invocation lost stdout/session: %+v", result)
	}
	if !reflect.DeepEqual(result.Sessions, []string{"selector-session", "failed-session"}) {
		t.Fatalf("failed improvement sessions = %q", result.Sessions)
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
	if want := "godel: improvement: yah-session: selector-sessiongodel: improvement: diagnostic detail\ngodel: improvement: yah-session: failed-session\n"; string(diagnostics) != want {
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
	root := routerNode(t, input, `{"branch":0,"text":"not the leaf input"}`, "root",
		Branch{When: "selected", Then: leaf})
	root.Env = append(root.Env, "GODEL_TEST_CWD="+info.Source)
	beforeRoot, beforeLeaf := *root, *leaf
	beforeRoot.Args = append([]string(nil), root.Args...)
	beforeRoot.Env = append([]string(nil), root.Env...)
	beforeRoot.Branches = append([]Branch(nil), root.Branches...)
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
	if got.Result != (Result{Text: input, Session: "leaf-session"}) || got.Stopped {
		t.Fatalf("improvement result = %+v", got)
	}
	if !reflect.DeepEqual(got.Sessions, []string{"root", "leaf-session"}) {
		t.Fatalf("improvement sessions = %q", got.Sessions)
	}
	if len(got.Invocations) != 2 {
		t.Fatalf("improvement invocations = %+v", got.Invocations)
	}
	selector, final := got.Invocations[0], got.Invocations[1]
	if selector.BranchIndex == nil || *selector.BranchIndex != 0 || selector.Error != "" {
		t.Fatalf("improvement selector observation = %+v", selector)
	}
	if final.BranchIndex != nil || final.Error != "" {
		t.Fatalf("improvement leaf observation = %+v", final)
	}
	if !reflect.DeepEqual(*root, beforeRoot) || !reflect.DeepEqual(*leaf, beforeLeaf) {
		t.Fatal("improvement mutated caller nodes")
	}
	diagnostics, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "godel: improvement: yah-session: root\ngodel: improvement: yah-session: leaf-session\n"
	if string(diagnostics) != want || sessionID(string(diagnostics)) != "" {
		t.Fatalf("improvement diagnostics contaminate work session: %q, want %q", diagnostics, want)
	}
}

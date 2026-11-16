package godel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestRunRecordHistory(t *testing.T) {
	p := &program{
		ID:  "AbcDefG",
		Dir: t.TempDir(),
	}
	cwd := t.TempDir()
	r, err := newRunRecord(p, "Version", "original\n", cwd)
	if err != nil {
		t.Fatal(err)
	}
	r.Status = "completed"
	r.FinalResponse = "output\n"
	r.Invocations = []metadata{{
		Elapsed: time.Millisecond,
	}}
	r.ImprovementInvocations = []metadata{
		{Elapsed: time.Millisecond},
		{Error: "executable not found"},
	}
	if err := r.save(p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.path(p))
	if err != nil {
		t.Fatal(err)
	}
	var saved runRecord
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID != r.ID || saved.Status != "completed" || saved.Input != "original\n" || saved.FinalResponse != "output\n" || saved.Directory != cwd || saved.Version != "Version" || saved.ProgramID != p.ID {
		t.Fatalf("lost history: %+v", saved)
	}
	if !reflect.DeepEqual(saved.Invocations, r.Invocations) || !reflect.DeepEqual(saved.ImprovementInvocations, r.ImprovementInvocations) {
		t.Fatalf("lost work/auxiliary trace: %+v", saved)
	}
	// Old branch fields are ordinary unknown JSON fields. They must not prevent
	// the saved response from supplying the next task.
	legacy := `{"final_response":"old answer", "stop_condition":"finished", "invocations":[{"elapsed_ns":1,"branch_index":0}]}`
	if err := os.WriteFile(r.path(p), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if response, err := latestFinalResponse(p); err != nil || response != "old answer" {
		t.Fatalf("old history response = %q, %v", response, err)
	}
	// Creating a fresh record with explicit input does not read old history.
	if err := os.WriteFile(r.path(p), []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := newRunRecord(p, "NewVers", "new task", cwd)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == r.ID || fresh.Input != "new task" || fresh.Version != "NewVers" {
		t.Fatalf("fresh run inherited history: %+v", fresh)
	}
}

func TestRunRecordIDs(t *testing.T) {
	p := &program{
		ID:  "AbcDefG",
		Dir: t.TempDir(),
	}
	if err := os.Mkdir(filepath.Join(p.Dir, "runs"), 0700); err != nil {
		t.Fatal(err)
	}
	// An old filename does not constrain new run IDs.
	previous := filepath.Join(p.Dir, "runs", "ffffffff-ffff-7000-8000-000000000000.json")
	if err := os.WriteFile(previous, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	cwd := t.TempDir()
	for range 5 {
		c, err := newRunRecord(p, "Incumbe", "task", cwd)
		if err != nil {
			t.Fatal(err)
		}
		id, err := uuid.Parse(c.ID)
		if err != nil {
			t.Fatalf("invalid UUID %q: %v", c.ID, err)
		}
		if id.String() != c.ID || id[6]>>4 != 7 || id[8]>>6 != 2 {
			t.Fatalf("not a canonical UUIDv7: %q", c.ID)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate run ID: %q", c.ID)
		}
		seen[c.ID] = true
	}
	for id := range seen {
		data, err := os.ReadFile(filepath.Join(p.Dir, "runs", id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var saved runRecord
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.ID != id || saved.ProgramID != p.ID || saved.Version != "Incumbe" || saved.Input != "task" || saved.Directory != cwd || saved.Status != "running" {
			t.Fatalf("lost history: %+v", saved)
		}
	}
	data, err := os.ReadFile(previous)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("old history changed: %q", data)
	}
}

func TestLatestFinalResponse(t *testing.T) {
	for _, status := range []string{"failed", "completed", "stopped", "running"} {
		t.Run(status, func(t *testing.T) {
			p := &program{ID: "AbcDefG", Dir: t.TempDir()}
			if err := os.Mkdir(filepath.Join(p.Dir, "runs"), 0700); err != nil {
				t.Fatal(err)
			}
			// Write out of filename order;
			// response recency is not write order.
			for _, r := range []runRecord{
				{ID: "00000000-0003-7000-8000-000000000000", FinalResponse: " \n\t\u2003"},
				{ID: "00000000-0002-7000-8000-000000000000", Status: status, FinalResponse: " \tlatest\x00\n "},
				{ID: "00000000-0001-7000-8000-000000000000", FinalResponse: "older"},
				{ID: "00000000-0004-7000-8000-000000000000"},
			} {
				if err := r.save(p); err != nil {
					t.Fatal(err)
				}
			}
			// Non-record files do not participate in the scan.
			if err := os.WriteFile(filepath.Join(p.Dir, "runs", "z.tmp"), []byte("not JSON"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := latestFinalResponse(p)
			if err != nil || got != " \tlatest\x00\n " {
				t.Fatalf("latest response = %q, %v", got, err)
			}
		})
	}
}

func TestLatestFinalResponseErrors(t *testing.T) {
	for _, mode := range []string{"missing directory", "empty directory", "blank responses", "malformed record", "unreadable record", "unreadable directory"} {
		t.Run(mode, func(t *testing.T) {
			p := &program{ID: "AbcDefG", Dir: t.TempDir()}
			dir := filepath.Join(p.Dir, "runs")
			if mode == "unreadable directory" {
				if err := os.WriteFile(dir, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else if mode != "missing directory" {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			want := "no saved nonblank final_response"
			switch mode {
			case "blank responses":
				if err := (&runRecord{ID: "old", FinalResponse: " \n\t"}).save(p); err != nil {
					t.Fatal(err)
				}
			case "malformed record", "unreadable record":
				if err := (&runRecord{ID: "older", FinalResponse: "must not hide corruption"}).save(p); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "z.json")
				if mode == "malformed record" {
					if err := os.WriteFile(path, []byte("not JSON"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink(filepath.Join(p.Dir, "missing"), path); err != nil {
					t.Fatal(err)
				}
				want = path
			case "unreadable directory":
				want = dir
			}
			got, err := latestFinalResponse(p)
			if err == nil || got != "" || !strings.Contains(err.Error(), want) {
				t.Fatalf("latest response = %q, %v; want error containing %q", got, err, want)
			}
		})
	}
}

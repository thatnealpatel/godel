package godel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"uuid"
)

func newRunRecord(p *program, input, directory string) (*runRecord, error) {
	if err := os.MkdirAll(filepath.Join(p.Dir, "runs"), 0700); err != nil {
		return nil, err
	}
	c := &runRecord{
		ID:        uuid.NewV7().String(),
		ProgramID: p.ID,
		Input:     input,
		Directory: directory,
		Status:    "running",
	}
	if err := c.save(p); err != nil {
		return nil, err
	}
	return c, nil
}

// latestFinalResponse scans history under the program
// lock, before creating the new run record. Descending
// UUIDv7 filenames give practical recency; status does
// not determine whether a saved response can be used
// as a task.
func latestFinalResponse(p *program) (string, error) {
	dir := filepath.Join(p.Dir, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("godel: read run history %q: %w", dir, err)
	}
	// ReadDir returns entries sorted by filename.
	for i := len(entries) - 1; i >= 0; i-- {
		name := entries[i].Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("godel: read run record %q: %w", path, err)
		}
		var r runRecord
		if err := json.Unmarshal(data, &r); err != nil {
			return "", fmt.Errorf("godel: decode run record %q: %w", path, err)
		}
		if strings.TrimSpace(r.FinalResponse) != "" {
			return r.FinalResponse, nil
		}
	}
	return "", fmt.Errorf("godel: no saved nonblank final_response available for program %q; supply a task on stdin", p.ID)
}

// runRecord retains history, including a
// possible task for a fresh invocation.
// A new invocation never restores an
// earlier run's execution state or
// identity.
type runRecord struct {
	ID                     string     `json:"id"`
	ProgramID              string     `json:"program_id"`
	Input                  string     `json:"input"`
	Directory              string     `json:"directory"`
	Status                 string     `json:"status"`
	FinalResponse          string     `json:"final_response"`
	Stopped                bool       `json:"stopped"`
	Invocations            []metadata `json:"invocations"`
	ImprovementInvocations []metadata `json:"improvement_invocations,omitempty"`
	ImprovementResponse    string     `json:"improvement_response"`
	ImprovementError       string     `json:"improvement_error"`
	Failure                string     `json:"failure"`
}

func (c *runRecord) path(p *program) string {
	return filepath.Join(p.Dir, "runs", c.ID+".json")
}

func (c *runRecord) save(p *program) error {
	return atomicJSON(c.path(p), c)
}

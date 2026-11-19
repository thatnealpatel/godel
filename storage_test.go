package godel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestStorageRegistration(t *testing.T) {
	home := storageTestHome(t)
	source := storageTestSource(t, home)
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, alias)
	if err != nil {
		t.Fatal(err)
	}
	p, err := registerProgram(relative)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z]{7}$`).MatchString(p.ID) {
		t.Fatalf("invalid ID %q", p.ID)
	}
	if p.Source != source || p.Dir != filepath.Join(home, ".godel", p.ID) {
		t.Fatalf("unexpected registration: %+v", p)
	}
	storageWriteFile(t, filepath.Join(source, "untracked.txt"), "edited", 0644)
	again, err := registerProgram(alias)
	if err != nil {
		t.Fatal(err)
	}
	if *again != *p {
		t.Fatalf("registration changed across edit: %+v, %+v", p, again)
	}
	manifestPath := filepath.Join(home, ".godel", "manifest.json")
	var manifest map[string]string
	if err := json.Unmarshal([]byte(storageReadFile(t, manifestPath)), &manifest); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest, map[string]string{source: p.ID}) {
		t.Fatalf("manifest = %v", manifest)
	}
	found, err := registerProgram(source)
	if err != nil || found == nil || *found != *p {
		t.Fatalf("lookup = %+v, %v", found, err)
	}

	// Moving the source and updating its
	// manifest key must retain identity.
	moved := filepath.Join(home, "moved")
	if err := os.Rename(source, moved); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{moved: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	storageWriteFile(t, manifestPath, string(data), 0600)
	found, err = registerProgram(moved)
	if err != nil {
		t.Fatal(err)
	}
	if found.Source != moved || found.ID != p.ID || found.Dir != p.Dir {
		t.Fatalf("lookup after move = %+v", found)
	}
}

func storageTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func storageTestSource(t *testing.T, home string) string {
	t.Helper()
	source := filepath.Join(home, "project")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	return source
}

func storageWriteFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func storageReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStorageConcurrentRegistration(t *testing.T) {
	home := storageTestHome(t)
	const count = 12
	sources := make([]string, count)
	for i := range sources {
		sources[i] = filepath.Join(home, fmt.Sprintf("source-%d", i))
		if err := os.Mkdir(sources[i], 0755); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan *program, count*2)
	errors := make(chan error, count*2)
	for _, source := range sources {
		for range 2 {
			wg.Go(func() {
				p, err := registerProgram(source)
				if err != nil {
					errors <- err
					return
				}
				results <- p
			})
		}
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	ids := make(map[string]string)
	for p := range results {
		if id, exists := ids[p.Source]; exists && id != p.ID {
			t.Errorf("source %q registered twice: %q and %q", p.Source, id, p.ID)
		}
		ids[p.Source] = p.ID
	}
	var manifest map[string]string
	if err := json.Unmarshal([]byte(storageReadFile(t, filepath.Join(home, ".godel", "manifest.json"))), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != count || !reflect.DeepEqual(manifest, ids) {
		t.Fatalf("lost registration: manifest %v, registrations %v", manifest, ids)
	}
}

func TestStorageManifestErrors(t *testing.T) {
	home := storageTestHome(t)
	source := storageTestSource(t, home)
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(home, ".godel", "manifest.json")
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		manifest map[string]string
		id       string
		want     string
	}{
		{
			"missing",
			map[string]string{source: p.ID},
			"ZZZZZZZ",
			"0 source mappings",
		},
		{
			"duplicate",
			map[string]string{source: p.ID, alias: p.ID},
			p.ID,
			"2 source mappings",
		},
		{
			"symlink",
			map[string]string{alias: p.ID},
			p.ID,
			"not canonical",
		},
		{
			"relative",
			map[string]string{".": p.ID},
			p.ID,
			"not canonical",
		},
		{
			"missing source",
			map[string]string{filepath.Join(home, "missing"): p.ID},
			p.ID,
			"resolve source",
		},
		{
			"invalid ID",
			map[string]string{source: "../oops"},
			"../oops",
			"invalid program ID",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.manifest)
			if err != nil {
				t.Fatal(err)
			}
			storageWriteFile(t, manifestPath, string(data), 0600)
			if _, err := programFromManifest(filepath.Join(home, ".godel"), test.manifest, test.id); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("lookup error = %v; want %q", err, test.want)
			}
		})
	}
	for _, contents := range []string{"{", "null", `{"path": 42}`} {
		storageWriteFile(t, manifestPath, contents, 0600)
		if _, err := registerProgram(source); err == nil {
			t.Fatalf("registered with malformed manifest %q", contents)
		}
	}
	if _, err := registerProgram(filepath.Join(home, "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source error = %v", err)
	}
	storageWriteFile(t, filepath.Join(source, "main.go"), "package main", 0600)
	if _, err := registerProgram(filepath.Join(source, "main.go")); err == nil {
		t.Fatal("registered a file instead of a source directory")
	}
}

func TestStorageShortID(t *testing.T) {
	for _, test := range []struct {
		id   string
		want bool
	}{
		{
			"AbCdEfG",
			true,
		},
		{
			"abcdefg",
			true,
		},
		{
			"ABCDEFG",
			true,
		},
		{
			"",
			false,
		},
		{
			"abcdef",
			false,
		},
		{
			"abcdefgh",
			false,
		},
		{
			"abcdef1",
			false,
		},
		{
			"abcdef/",
			false,
		},
		{
			"abcdefé",
			false,
		},
		{
			"abcdefg\n",
			false,
		},
	} {
		if got := shortIDPattern.MatchString(test.id); got != test.want {
			t.Errorf("shortIDPattern.MatchString(%q) = %v; want %v", test.id, got, test.want)
		}
	}
}

func TestStorageAtomicJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := atomicJSON(path, map[string]string{"value": "first"}); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, map[string]string{"value": "second"}); err != nil {
		t.Fatal(err)
	}
	before := storageReadFile(t, path)
	var value map[string]string
	if err := json.Unmarshal([]byte(before), &value); err != nil || value["value"] != "second" {
		t.Fatalf("replacement = %q, %v", before, err)
	}
	if err := atomicJSON(path, make(chan int)); err == nil {
		t.Fatal("encoded an unsupported JSON value")
	}
	if got := storageReadFile(t, path); got != before {
		t.Fatalf("failed encoding damaged old state: %q", got)
	}
	targetDir := filepath.Join(dir, "directory")
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(targetDir, "cannot replace a directory"); err == nil {
		t.Fatal("replaced a directory with JSON")
	}
	if err := atomicJSON(filepath.Join(dir, "absent", "state.json"), "value"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("temporary JSON files leaked: %v", entries)
	}
}

func TestRegistrationLeavesNotesAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	source := t.TempDir()
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("registration seeded files: %v", entries)
	}
	path := filepath.Join(p.Dir, "improvement.md")
	const notes = "Existing notes\n"
	storageWriteFile(t, path, notes, 0600)
	again, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	if *again != *p {
		t.Fatalf("registration changed: %+v != %+v", again, p)
	}
	if got := storageReadFile(t, path); got != notes {
		t.Fatalf("registration changed existing notes: %q", got)
	}
}

package godel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GOTOOLCHAIN", "local")
	// Go's telemetry sidecar can outlive the command and recreate files while
	// TempDir removes HOME. Disable it within this fixture's private config.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "go", "telemetry", "off").CombinedOutput(); err != nil {
		t.Fatalf("disable fixture telemetry: %v\n%s", err, output)
	}
	// Published archives are deliberately
	// read-only. Restore directory write
	// permissions only for the test
	// framework's eventual temporary-tree
	// cleanup.
	t.Cleanup(func() {
		if err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		}); err != nil {
			t.Errorf("prepare test cleanup: %v", err)
		}
	})
	return home
}

func storageTestSource(t *testing.T, home string) string {
	t.Helper()
	source := filepath.Join(home, "project")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	storageWriteFile(t, filepath.Join(source, "go.mod"), "module example.test/program\n\ngo 1.27.3\n", 0644)
	storageWriteFile(t, filepath.Join(source, "main.go"), `package main
import (
    "embed"
    "fmt"
)
//go:embed message.txt
var inputs embed.FS
func main() {
    message, err := inputs.ReadFile("message.txt")
    if err != nil { panic(err) }
    fmt.Print(string(message))
}
`, 0644)
	storageWriteFile(t, filepath.Join(source, "message.txt"), "original live input\n", 0644)
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

func TestStorageArchiveRetainsLiveInputs(t *testing.T) {
	home := storageTestHome(t)
	source := storageTestSource(t, home)
	storageWriteFile(t, filepath.Join(source, "script.sh"), "#!/bin/sh\necho original\n", 0751)
	storageWriteFile(t, filepath.Join(source, "godel"), "ordinary source file named godel", 0644)
	for _, name := range []string{".git", ".jj"} {
		if err := os.Mkdir(filepath.Join(source, name), 0700); err != nil {
			t.Fatal(err)
		}
		storageWriteFile(t, filepath.Join(source, name, "metadata"), "not a build input", 0600)
	}
	external := filepath.Join(home, "external.txt")
	storageWriteFile(t, external, "external original", 0640)
	if err := os.Symlink(external, filepath.Join(source, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(home, "external-directory")
	if err := os.Mkdir(externalDir, 0755); err != nil {
		t.Fatal(err)
	}
	storageWriteFile(t, filepath.Join(externalDir, "input.txt"), "directory original", 0644)
	if err := os.Symlink(externalDir, filepath.Join(source, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id, err := archive(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.Dir, "src", id)
	if !regexp.MustCompile(`^[A-Za-z]{7}$`).MatchString(id) {
		t.Fatalf("invalid archive ID %q", id)
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0222 != 0 {
			t.Errorf("mutable archive entry %q: %s", path, info.Mode())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", ".jj"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("metadata %q retained: %v", name, err)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "script.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0551 {
		t.Fatalf("script permissions = %o; want 551", got)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	storageWriteFile(t, external, "external changed", 0640)
	storageWriteFile(t, filepath.Join(externalDir, "input.txt"), "directory changed", 0644)
	for name, want := range map[string]string{
		"message.txt":          "original live input\n",
		"linked.txt":           "external original",
		"linked-dir/input.txt": "directory original",
		"script.sh":            "#!/bin/sh\necho original\n",
		"godel":                "ordinary source file named godel",
	} {
		if got := storageReadFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("retained %s = %q; want %q", name, got, want)
		}
	}

	// A subsequent version comes from
	// current live source, not the previous
	// archive.
	source = storageTestSource(t, home)
	storageWriteFile(t, filepath.Join(source, "message.txt"), "new live input\n", 0644)
	next, err := archive(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if next == id {
		t.Fatal("archive reused an existing version ID")
	}
	if got := storageReadFile(t, filepath.Join(p.Dir, "src", next, "message.txt")); got != "new live input\n" {
		t.Fatalf("new version ignored live edits: %q", got)
	}
	// Archives retain information, including a failed experiment; they do not
	// compile or activate source.
	storageWriteFile(t, filepath.Join(source, "main.go"), "this is not Go", 0644)
	invalid, err := archive(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := storageReadFile(t, filepath.Join(p.Dir, "src", invalid, "main.go")); got != "this is not Go" {
		t.Fatalf("lost failed experiment: %q", got)
	}
}

func TestStorageArchiveExcludesStorage(t *testing.T) {
	home := storageTestHome(t)
	// A source containing ~/.godel must not
	// recursively archive its output.
	source := storageTestSource(t, home)
	for _, name := range []string{"go.mod", "main.go", "message.txt"} {
		if err := os.Rename(filepath.Join(source, name), filepath.Join(home, name)); err != nil {
			t.Fatal(err)
		}
	}
	p, err := registerProgram(home)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id, err := archive(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "src", id, ".godel")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive contains storage: %v", err)
	}
}

func TestStorageArchiveErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
		want string
	}{
		{
			"dangling link",
			func(source string) error {
				return os.Symlink("missing", filepath.Join(source, "link"))
			},
			"resolve input",
		},
		{
			"directory cycle",
			func(source string) error {
				return os.Symlink(source, filepath.Join(source, "cycle"))
			},
			"cycle",
		},
		{
			"special file",
			func(source string) error {
				return syscall.Mkfifo(filepath.Join(source, "fifo"), 0600)
			},
			"unsupported source file",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := storageTestHome(t)
			source := storageTestSource(t, home)
			p, err := registerProgram(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.make(source); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := archive(ctx, p); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("archive error = %v; want %q", err, test.want)
			}
			entries, err := os.ReadDir(filepath.Join(p.Dir, "src"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("unpublished archive leaked: %v", entries)
			}
		})
	}
	home := storageTestHome(t)
	source := storageTestSource(t, home)
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := archive(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled archive error = %v", err)
	}
	if _, err := archive(context.Background(), nil); err == nil {
		t.Fatal("archived a nil program")
	}
	if err := os.Symlink(filepath.Dir(p.Dir), filepath.Join(source, "storage-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := archive(context.Background(), p); err == nil || !strings.Contains(err.Error(), "archive storage") {
		t.Fatalf("link into storage error = %v", err)
	}
}

func TestStorageArchiveNeedsNoBuildContext(t *testing.T) {
	home := storageTestHome(t)
	source := filepath.Join(home, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	storageWriteFile(t, filepath.Join(source, "notes.txt"), "source information", 0600)
	t.Setenv("GOWORK", filepath.Join(home, "absent.work"))
	t.Setenv("PATH", "")
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	id, err := archive(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got := storageReadFile(t, filepath.Join(p.Dir, "src", id, "notes.txt")); got != "source information" {
		t.Fatalf("retained notes = %q", got)
	}
}

func TestInitialLearningMaterialsCanEvolve(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	source := t.TempDir()
	p, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.Dir, "improvement.md")
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(initial), source) {
		t.Fatalf("initial learning materials omit live source: %s", initial)
	}
	if err := os.Rename(path, filepath.Join(p.Dir, "research-notes.md")); err != nil {
		t.Fatal(err)
	}
	again, err := registerProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != p.ID {
		t.Fatalf("registration changed identity: %s != %s", again.ID, p.ID)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("registration recreated removed learning entry point: %v", err)
	}
}

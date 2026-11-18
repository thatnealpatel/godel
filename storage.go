package godel

import (
	_ "embed"
	"encoding/json"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
)

func registerProgram(source string) (*program, error) {
	source, err := canonicalSource(source)
	if err != nil {
		return nil, err
	}
	root, lock, err := lockManifest()
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	manifest, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	if id, ok := manifest[source]; ok {
		return programFromManifest(root, manifest, id)
	}
	id := lowEntropyID()
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("create program %q: %w", id, err)
	}
	// Seed learning once, before publication. Later
	// nodes may reorganize or replace this entry
	// point; registration never recreates it on reuse.
	learning := fmt.Sprintf(learningTemplate, source)
	if err := os.WriteFile(filepath.Join(dir, "improvement.md"), []byte(learning), 0600); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("initialize learning materials: %w", err)
	}
	manifest[source] = id
	if err := atomicJSON(filepath.Join(root, "manifest.json"), manifest); err != nil {
		// The directory has not been published in the manifest.
		os.RemoveAll(dir)
		return nil, err
	}
	return &program{
		ID:     id,
		Source: source,
		Dir:    dir,
	}, nil
}

type program struct {
	ID, Source, Dir string
}

func canonicalSource(source string) (string, error) {
	path, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("absolute source path %q: %w", source, err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve source %q: %w", source, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat source %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source %q is not a directory", path)
	}
	return path, nil
}

func readManifest(root string) (map[string]string, error) {
	path := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest %q: %w", path, err)
	}
	if manifest == nil {
		return nil, fmt.Errorf("manifest %q must be a JSON object", path)
	}
	return manifest, nil
}

func programFromManifest(root string, manifest map[string]string, id string) (*program, error) {
	if !shortIDPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid program ID %q in manifest", id)
	}
	var source string
	count := 0
	for path, mapped := range manifest {
		if mapped == id {
			source = path
			count++
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("program %q has %d source mappings; want exactly one", id, count)
	}
	canonical, err := canonicalSource(source)
	if err != nil {
		return nil, err
	}
	if canonical != source {
		return nil, fmt.Errorf("manifest source %q is not canonical (want %q)", source, canonical)
	}
	dir := filepath.Join(root, id)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat program directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("program path %q is not a directory", dir)
	}
	return &program{
		ID:     id,
		Source: source,
		Dir:    dir,
	}, nil
}

var shortIDPattern = regexp.MustCompile(`^[A-Za-z]{7}$`)

// lowEntropyID returns a pseudorandom
// identifier with approximately 40 bits
// of entropy.
func lowEntropyID() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	id := make([]byte, 7)
	for i := range id {
		id[i] = alphabet[mathrand.IntN(len(alphabet))]
	}
	return string(id)
}

// atomicJSON replaces path only after the
// complete JSON has reached its same-directory
// temporary file's backing store.
func atomicJSON(path string, value any) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary JSON for %q: %w", path, err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := json.NewEncoder(file).Encode(value); err != nil {
		return fmt.Errorf("encode JSON for %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync JSON for %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close JSON for %q: %w", path, err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("publish JSON %q: %w", path, err)
	}
	return nil
}

//go:embed templates/learning.md
var learningTemplate string

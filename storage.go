package godel

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func archive(ctx context.Context, p *program) (string, error) {
	if p == nil {
		return "", fmt.Errorf("archive: nil program")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("archive source: %w", err)
	}
	source, err := canonicalSource(p.Source)
	if err != nil {
		return "", err
	}
	versions := filepath.Join(p.Dir, "src")
	if err := os.MkdirAll(versions, 0700); err != nil {
		return "", fmt.Errorf("create archive storage %q: %w", versions, err)
	}
	storage, err := filepath.EvalSymlinks(filepath.Dir(p.Dir))
	if err != nil {
		return "", fmt.Errorf("resolve archive storage: %w", err)
	}
	storage, err = filepath.Abs(storage)
	if err != nil {
		return "", fmt.Errorf("absolute archive storage: %w", err)
	}
	if withinStorage(source, storage) {
		return "", fmt.Errorf("source %q is inside archive storage %q", source, storage)
	}
	id := lowEntropyID()
	dir := filepath.Join(versions, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", fmt.Errorf("create archive %q: %w", id, err)
	}
	published := false
	defer func() {
		if !published {
			removeUnpublishedArchive(dir)
		}
	}()
	if err := copySource(ctx, source, dir, storage, true, make(map[string]bool)); err != nil {
		return "", fmt.Errorf("copy source into archive %q: %w", id, err)
	}
	if err := freezeArchive(dir); err != nil {
		return "", fmt.Errorf("freeze archive %q: %w", id, err)
	}
	published = true
	return id, nil
}

func withinStorage(path, storage string) bool {
	rel, err := filepath.Rel(storage, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// This is only used before publication,
// including after a partially completed
// freeze. Published archives must never
// be made writable or removed.
func removeUnpublishedArchive(path string) {
	filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0700)
		}
		return nil
	})
	os.RemoveAll(path)
}

// copySource dereferences links, including directory links, so the retained
// version has no dependency on the live tree. The ancestor set detects cycles
// without rejecting independent links to the same input.
func copySource(ctx context.Context, source, dest, storage string, root bool, ancestors map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve input %q: %w", source, err)
	}
	if withinStorage(resolved, storage) {
		return fmt.Errorf("input %q points into archive storage", source)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("stat input %q: %w", source, err)
	}
	if info.IsDir() {
		if ancestors[resolved] {
			return fmt.Errorf("symlink directory cycle at %q", source)
		}
		ancestors[resolved] = true
		defer delete(ancestors, resolved)
		if !root {
			if err := os.Mkdir(dest, info.Mode().Perm()|0700); err != nil {
				return fmt.Errorf("create archive directory %q: %w", dest, err)
			}
		}
		entries, err := os.ReadDir(resolved)
		if err != nil {
			return fmt.Errorf("read source directory %q: %w", source, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == ".git" || name == ".jj" {
				continue
			}
			input := filepath.Join(resolved, name)
			// Skip the storage tree itself when
			// the live source is an ancestor (for
			// example HOME). Links into storage
			// are errors instead.
			if input == storage {
				continue
			}
			if err := copySource(ctx, input, filepath.Join(dest, name), storage, false, ancestors); err != nil {
				return err
			}
		}
		mode := info.Mode().Perm()
		if err := os.Chmod(dest, mode); err != nil {
			return fmt.Errorf("set directory permissions %q: %w", dest, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported source file %q (mode %s)", source, info.Mode())
	}
	input, err := os.Open(resolved)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", source, err)
	}
	defer input.Close()
	output, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create archive file %q: %w", dest, err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy source file %q: %w", source, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close archive file %q: %w", dest, closeErr)
	}
	if err := os.Chmod(dest, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set file permissions %q: %w", dest, err)
	}
	return nil
}

func freezeArchive(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := freezeArchive(filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
	}
	return os.Chmod(path, info.Mode().Perm()&^0222)
}

package app

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type failingMetadataFile struct {
	metadataFile
	phase string
	cause error
}

func (f failingMetadataFile) Write(data []byte) (int, error) {
	if f.phase == "write" || f.phase == "short-write" {
		n, err := f.metadataFile.Write(data[:len(data)/2])
		if f.phase == "write" {
			return n, errors.Join(err, f.cause)
		}
		return n, err
	}
	return f.metadataFile.Write(data)
}

func (f failingMetadataFile) Sync() error {
	if f.phase == "file-sync" {
		return f.cause
	}
	return f.metadataFile.Sync()
}

func (f failingMetadataFile) Close() error {
	err := f.metadataFile.Close()
	if f.phase == "close" {
		return errors.Join(err, f.cause)
	}
	return err
}

func TestMetadataReplacementNeverTruncatesTheValidCopy(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"create", "write", "short-write", "file-sync", "close", "rename", "directory-sync"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			old, next := `{"complete":"old"}`, `{"complete":"new"}`
			files := realMetadataFiles()
			if err := files.replace(path, []byte(old)); err != nil {
				t.Fatal(err)
			}
			cause := errors.New("injected " + phase)
			realCreate := files.createTemp
			files.createTemp = func(dir, pattern string) (metadataFile, error) {
				if phase == "create" {
					return nil, cause
				}
				file, err := realCreate(dir, pattern)
				if err != nil {
					return nil, err
				}
				return failingMetadataFile{metadataFile: file, phase: phase, cause: cause}, nil
			}
			if phase == "rename" {
				files.rename = func(string, string) error { return cause }
			}
			if phase == "directory-sync" {
				files.syncDir = func(string) error { return cause }
			}
			err := files.replace(path, []byte(next))
			wantErr := cause
			if phase == "short-write" {
				wantErr = io.ErrShortWrite
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("lost failure: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if phase == "directory-sync" {
				want = next
			}
			if !json.Valid(data) || string(data) != want {
				t.Fatalf("valid copy damaged: %s, want %s", data, want)
			}
			temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".state.json.tmp-*"))
			if err != nil || len(temps) != 0 {
				t.Fatalf("stale temporary files: %v, %v", temps, err)
			}
		})
	}
}

func TestMetadataReplacementPermissionsAndDirectoryDurability(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir := filepath.Join(base, "new", "stack")
	path := filepath.Join(dir, "state.json")
	files := realMetadataFiles()
	var synced []string
	files.syncDir = func(path string) error {
		synced = append(synced, path)
		return syncMetadataDirectory(path)
	}
	if err := files.replace(path, []byte(`{"valid":true}`)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(synced, []string{base, filepath.Join(base, "new"), dir}) {
		t.Fatalf("directory entries were not persisted: %v", synced)
	}
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{path, 0o600}, {dir, 0o700}} {
		info, err := os.Stat(tc.path)
		if err != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("permissions for %s: %v, %v", tc.path, info, err)
		}
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := files.replace(path, []byte(`{"replacement":true}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("replacement did not restore private permissions: %v, %v", info, err)
	}
}

func TestMetadataTemporaryCleanupFailureIsReported(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	files := realMetadataFiles()
	if err := files.replace(path, []byte(`{"old":true}`)); err != nil {
		t.Fatal(err)
	}
	renameErr, cleanupErr := errors.New("rename failure"), errors.New("cleanup failure")
	files.rename = func(string, string) error { return renameErr }
	files.remove = func(string) error { return cleanupErr }
	err := files.replace(path, []byte(`{"new":true}`))
	if !errors.Is(err, renameErr) || !errors.Is(err, cleanupErr) || !strings.Contains(err.Error(), ".state.json.tmp-") {
		t.Fatalf("cleanup failure/path was hidden: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"old":true}` {
		t.Fatalf("cleanup failure damaged original: %s, %v", data, err)
	}
}

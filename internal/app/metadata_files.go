package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type metadataFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

// The real filesystem and failure-injecting tests use the same narrow seam.
type metadataFiles struct {
	createTemp func(string, string) (metadataFile, error)
	rename     func(string, string) error
	remove     func(string) error
	syncDir    func(string) error
}

func realMetadataFiles() metadataFiles {
	return metadataFiles{
		createTemp: func(dir, pattern string) (metadataFile, error) { return os.CreateTemp(dir, pattern) },
		rename:     os.Rename, remove: os.Remove, syncDir: syncMetadataDirectory,
	}
}

func syncMetadataDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func (fs metadataFiles) ensureDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("metadata directory %s is not a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := fs.ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	// Persist the new directory's entry as well as files subsequently put in it.
	return fs.syncDir(filepath.Dir(path))
}

// replace syncs a private same-directory temporary file, atomically renames it,
// then syncs the directory. A post-rename sync error means the new file is
// visible but its crash durability is unconfirmed; it is not rolled back.
// Filesystems/platforms that cannot sync directories return an error rather
// than silently claiming durability. This is not a multi-file transaction.
func (fs metadataFiles) replace(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := fs.ensureDir(dir); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	file, err := fs.createTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	renamed := false
	defer func() {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		if !renamed {
			if removeErr := fs.remove(temp); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove temporary metadata %s: %w", temp, removeErr))
			}
		}
		if err != nil {
			err = fmt.Errorf("replace metadata %s: %w", path, err)
		}
	}()
	if n, err := file.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	closeErr := file.Close()
	file = nil
	if closeErr != nil {
		return closeErr
	}
	if err := fs.rename(temp, path); err != nil {
		return err
	}
	renamed = true
	return fs.syncDir(dir)
}

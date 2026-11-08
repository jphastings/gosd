package l4t

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// Cached returns the kit's members as [Kit.Cache] stored them under root.
// With no copy there, its error wraps [io/fs.ErrNotExist]. A copy with any
// member missing or not matching its pin is never used: Cached deletes it,
// and its error says why.
func (k Kit) Cached(root string) (Inputs, error) {
	dir := filepath.Join(root, k.SHA256)
	if _, err := os.Stat(dir); err != nil {
		return Inputs{}, err
	}
	var in Inputs
	for name, dst := range in.members() {
		b, err := os.ReadFile(filepath.Join(dir, path.Base(name)))
		if err != nil || !k.matches(name, b) {
			_ = os.RemoveAll(dir)
			return Inputs{}, fmt.Errorf("the cached %s in %s is missing or doesn't match the kit's, so that copy was deleted", path.Base(name), dir)
		}
		*dst = b
	}
	return in, nil
}

// Cache stores the kit's members under root, in a directory named for the
// kit's hash. A copy appears whole or not at all: the members are written
// and synced in a temporary directory that is then renamed into place, so a
// write cut short is never read back as complete.
func (k Kit) Cache(root string, in Inputs) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(root, ".partial-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for name, data := range in.members() {
		if err := writeSynced(filepath.Join(tmp, path.Base(name)), *data); err != nil {
			return err
		}
	}
	if err := syncPath(tmp); err != nil {
		return err
	}
	dir := filepath.Join(root, k.SHA256)
	// Replace any copy already here, which Cached didn't use.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return err
	}
	return syncPath(root)
}

// Prune removes everything under root but this kit's copy, best-effort:
// whatever can't be removed is left for next time.
func (k Kit) Prune(root string) {
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != k.SHA256 {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

func writeSynced(name string, data []byte) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func syncPath(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

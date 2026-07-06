package gadget

import (
	"io/fs"
	"os"
)

// writableFS is the minimal set of file operations Apply/Close need to
// materialize and tear down a configfs gadget tree. io/fs.FS is read-only,
// so this exists purely to give production code (osFS) and tests (the fake
// in fakes_test.go) a shared, writable seam — configfs itself is nothing
// but directories, attribute files, and symlinks, and every path Apply/
// Close builds is already absolute, so no root/chroot indirection is
// needed.
type writableFS interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(path string, data []byte, perm fs.FileMode) error
	Symlink(oldname, newname string) error
	Remove(path string) error
	ReadDir(path string) ([]fs.DirEntry, error)
}

// osFS implements writableFS directly against the real filesystem.
type osFS struct{}

func (osFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (osFS) Symlink(oldname, newname string) error      { return os.Symlink(oldname, newname) }
func (osFS) Remove(path string) error                   { return os.Remove(path) }
func (osFS) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }

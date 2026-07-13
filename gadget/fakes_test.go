package gadget

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// fakeOp records one call made against a fakeFS, in order, so tests can
// assert not just the end state but the sequence Apply/Close made it in
// (e.g. that UDC is unbound before anything is removed).
type fakeOp struct {
	kind, path string // kind is "mkdir", "write", "symlink", "remove", "readdir"
}

// fakeFS is an in-memory writableFS standing in for configfs, modeling just
// enough of its real semantics for Apply/Close to be tested without
// hardware: directories, plain attribute files, and symlinks are tracked
// separately, and — matching real configfs (see
// Documentation/usb/gadget_configfs.rst, whose teardown sequence never
// individually removes attribute files before rmdir'ing their directory) —
// Remove on a directory succeeds and cascade-deletes any plain files it
// directly contains, but fails if a child directory or symlink is still
// present: that's a genuine ordering bug, not something configfs papers
// over. The one child-directory exception is configfs "default groups",
// which the kernel creates and removes alongside their parent
// (f_mass_storage's lun.0 is the only one modeled here): those are created
// by MkdirAll on the function directory, refused by a direct Remove, and
// cascade-removed with their parent.
type fakeFS struct {
	dirs          map[string]bool
	files         map[string][]byte
	links         map[string]string
	defaultGroups map[string]bool
	calls         []fakeOp
}

func newFakeFS() *fakeFS {
	return &fakeFS{
		dirs:          map[string]bool{},
		files:         map[string][]byte{},
		links:         map[string]string{},
		defaultGroups: map[string]bool{},
	}
}

func (f *fakeFS) MkdirAll(path string, _ fs.FileMode) error {
	f.calls = append(f.calls, fakeOp{"mkdir", path})
	for p := path; p != "" && p != "/"; p = parentOf(p) {
		f.dirs[p] = true
	}
	if strings.HasPrefix(baseOf(path), "mass_storage.") && baseOf(parentOf(path)) == "functions" {
		lun := path + "/lun.0"
		f.dirs[lun] = true
		f.defaultGroups[lun] = true
	}
	return nil
}

func (f *fakeFS) WriteFile(path string, data []byte, _ fs.FileMode) error {
	f.calls = append(f.calls, fakeOp{"write", path})
	if !f.dirs[parentOf(path)] {
		return fmt.Errorf("fakeFS: WriteFile %s: %w: parent directory not created", path, fs.ErrNotExist)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	f.files[path] = cp
	return nil
}

func (f *fakeFS) Symlink(oldname, newname string) error {
	f.calls = append(f.calls, fakeOp{"symlink", newname})
	if !f.dirs[parentOf(newname)] {
		return fmt.Errorf("fakeFS: Symlink %s: %w: parent directory not created", newname, fs.ErrNotExist)
	}
	f.links[newname] = oldname
	return nil
}

func (f *fakeFS) Remove(path string) error {
	f.calls = append(f.calls, fakeOp{"remove", path})

	if f.defaultGroups[path] {
		return fmt.Errorf("fakeFS: Remove %s: %w: configfs default groups are removed with their parent, never directly", path, fs.ErrPermission)
	}
	if _, ok := f.links[path]; ok {
		delete(f.links, path)
		return nil
	}
	if _, ok := f.files[path]; ok {
		delete(f.files, path)
		return nil
	}
	if !f.dirs[path] {
		return fmt.Errorf("fakeFS: Remove %s: %w", path, fs.ErrNotExist)
	}

	prefix := path + "/"
	for d := range f.dirs {
		if strings.HasPrefix(d, prefix) && !f.defaultGroups[d] {
			return fmt.Errorf("fakeFS: Remove %s: directory not empty (subdirectory %s)", path, d)
		}
	}
	for l := range f.links {
		if strings.HasPrefix(l, prefix) {
			return fmt.Errorf("fakeFS: Remove %s: directory not empty (symlink %s)", path, l)
		}
	}
	for p := range f.files {
		if strings.HasPrefix(p, prefix) {
			delete(f.files, p)
		}
	}
	for d := range f.dirs {
		if strings.HasPrefix(d, prefix) {
			delete(f.dirs, d)
			delete(f.defaultGroups, d)
		}
	}
	delete(f.dirs, path)
	return nil
}

func (f *fakeFS) ReadDir(path string) ([]fs.DirEntry, error) {
	f.calls = append(f.calls, fakeOp{"readdir", path})
	if !f.dirs[path] {
		return nil, fmt.Errorf("fakeFS: ReadDir %s: %w", path, fs.ErrNotExist)
	}

	prefix := path + "/"
	seen := map[string]bool{}
	for d := range f.dirs {
		if name, ok := directChild(d, prefix); ok {
			seen[name] = true
		}
	}
	for p := range f.files {
		if name, ok := directChild(p, prefix); ok {
			seen[name] = true
		}
	}
	for l := range f.links {
		if name, ok := directChild(l, prefix); ok {
			seen[name] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]fs.DirEntry, len(names))
	for i, name := range names {
		entries[i] = fakeDirEntry(name)
	}
	return entries, nil
}

func directChild(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

func parentOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i <= 0 {
		return "/"
	}
	return path[:i]
}

func baseOf(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

// callsOfKind returns the paths of every recorded call of the given kind,
// in call order.
func (f *fakeFS) callsOfKind(kind string) []string {
	var paths []string
	for _, c := range f.calls {
		if c.kind == kind {
			paths = append(paths, c.path)
		}
	}
	return paths
}

// indexOfCall returns the position of the first call matching kind+path in
// f.calls, or -1 if it never happened — used to assert ordering (e.g. the
// UDC unbind happens before any remove).
func (f *fakeFS) indexOfCall(kind, path string) int {
	for i, c := range f.calls {
		if c.kind == kind && c.path == path {
			return i
		}
	}
	return -1
}

type fakeDirEntry string

func (e fakeDirEntry) Name() string      { return string(e) }
func (e fakeDirEntry) IsDir() bool       { return false }
func (e fakeDirEntry) Type() fs.FileMode { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("fakeDirEntry: Info not supported")
}

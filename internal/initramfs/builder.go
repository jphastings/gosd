package initramfs

import "io"

// Builder produces an initramfs archive from a Spec. It exists so
// internal/image can hold a reference to "the thing that builds the
// initramfs" without depending on how that's implemented.
type Builder interface {
	Build(w io.Writer, spec Spec) error
}

// DefaultBuilder is the production Builder: it delegates to Build.
type DefaultBuilder struct{}

// Build implements Builder.
func (DefaultBuilder) Build(w io.Writer, spec Spec) error {
	return Build(w, spec)
}

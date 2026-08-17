// Package boardset registers every board GoSD ships, and reports the set it
// registered.
//
// Registration used to live in cmd/gosd's init(), which meant boards.All()
// and boards.IDs() were only ever populated inside package main: nothing
// else - tests especially - could ask the registry what boards exist, so
// fleet-wide checks hand-maintained their own duplicate board lists and
// drifted. Importing this package (blank-importing it is enough) populates
// the registry.
package boardset

import (
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/cubiea5e"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pi3b"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/boards/rock4se"
)

func init() {
	boards.Register(pizero2w.New())
	boards.Register(pizerow.New())
	boards.Register(radxazero3e.New())
	// nanopi-zero2 is public: gosd-f39b's U-Boot artifact pipeline entries
	// are published in the artifacts/v0.2.0 release, so real
	// (non---artifacts-dir) fetches for this board now resolve.
	boards.Register(nanopizero2.New())
	// qemu-virt is internal-only (see CLAUDE.md's locked decision): it's a
	// real, fully buildable board, but only reachable via an explicit
	// --board=qemu-virt, never part of the default no---board build set,
	// --help text, or catalog generation.
	boards.RegisterInternal(qemuvirt.New())
	// rock-4se is public: its kernel and U-Boot (TF-A compiled from
	// source, no rkbin blobs) are published in the artifacts/v0.5.0
	// release, so real (non---artifacts-dir) fetches for this board now
	// resolve (bean gosd-h8a8's activation flip).
	boards.Register(rock4se.New())
	// pi-3b is public: its kernel and both family DTBs (one image covers
	// the 3B and the 3B+) are published in the artifacts/v0.8.0 release,
	// so real (non---artifacts-dir) fetches for this board now resolve
	// (bean gosd-7wv9's activation flip).
	boards.Register(pi3b.New())
	// cubie-a5e is public: its kernel (the fleet's first Allwinner member)
	// and U-Boot (BL31 compiled from a pinned TF-A fork, no rkbin-style
	// blobs) are published in the artifacts/v0.9.0 release, so real
	// (non---artifacts-dir) fetches for this board now resolve (bean
	// gosd-zh95's activation flip).
	boards.Register(cubiea5e.New())
}

// Registered returns every board this package registers - public and
// internal-only alike - sorted by Name(). Use it for checks that must cover
// the whole fleet; boards.All() and boards.IDs() deliberately omit
// internal-only boards, and the public subset is boards.IsInternal away.
//
// This is a thin wrapper around boards.AllIncludingInternal(), but it
// carries a real guarantee that function alone doesn't: calling
// boards.AllIncludingInternal() without importing this package returns an
// empty slice, which would make every check in internal/repocheck pass
// vacuously. Importing boardset (even blank-importing it, as its own doc
// comment says) is what populates the registry, so Registered() cannot
// return empty by accident the way a direct AllIncludingInternal() call
// could.
func Registered() []boards.Board {
	return boards.AllIncludingInternal()
}

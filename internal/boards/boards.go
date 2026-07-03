// Package boards is the registry of SD-card targets gosd knows how to build
// for. Every board GoSD supports is arm64; adding a board means adding an
// entry here and wiring its board-specific behaviour (kernel config, U-Boot,
// gadget config, ...) in the packages that consume this registry.
package boards

// Board identifies a single supported hardware target.
type Board struct {
	// ID is the stable, user-facing identifier used on the --board flag
	// and in output filenames (e.g. "pi-zero-2w").
	ID string
	// DisplayName is a human-readable label for logs and help text.
	DisplayName string
}

var registry = []Board{
	{ID: "pi-zero-2w", DisplayName: "Raspberry Pi Zero 2 W"},
	{ID: "radxa-zero-3e", DisplayName: "Radxa Zero 3E"},
}

// All returns every registered board, in a stable order.
func All() []Board {
	out := make([]Board, len(registry))
	copy(out, registry)
	return out
}

// Find looks up a board by its ID.
func Find(id string) (Board, bool) {
	for _, b := range registry {
		if b.ID == id {
			return b, true
		}
	}
	return Board{}, false
}

// IDs returns the IDs of every registered board, in the same order as All.
func IDs() []string {
	ids := make([]string, len(registry))
	for i, b := range registry {
		ids[i] = b.ID
	}
	return ids
}

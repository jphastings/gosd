package statusled

import (
	"fmt"
	"sync"
)

// Sysfs is the real implementation main.go wires against DefaultRoot,
// shaped to satisfy boot.Deps' StatusLED interface without this package
// needing to import the boot package at all.
//
// Discovery happens lazily, on the first Booting/Running/Fatal call, never
// at construction: /sys isn't mounted until gosd-init's own early mounts run
// (boot.Run's mountEarly, step 1), which happens after main.go builds
// boot.Deps but before Booting — the earliest of the three, called right
// after the console opens — can ever run. Once resolved the result is
// cached, so a board with no status LED pays for exactly one failed lookup
// per boot, not three.
type Sysfs struct {
	root string

	once sync.Once
	disc Discovery
	err  error
}

// New returns a Sysfs that will discover its LED under root on first use.
func New(root string) *Sysfs {
	return &Sysfs{root: root}
}

func (s *Sysfs) Booting() error { return s.apply(LED.Booting) }
func (s *Sysfs) Running() error { return s.apply(LED.Running) }
func (s *Sysfs) Fatal() error   { return s.apply(LED.Fatal) }

// Describe reports what discovery found, for the boot sequence to log once
// (see boot.StatusLEDDescriber). It resolves the LED if nothing has yet, so
// the answer is the same one the state methods act on rather than a second
// opinion.
func (s *Sysfs) Describe() string {
	s.resolve()
	if s.err != nil {
		return fmt.Sprintf("looking for an LED under %s failed: %v", s.root, s.err)
	}
	return s.disc.String()
}

func (s *Sysfs) resolve() {
	s.once.Do(func() {
		s.disc, s.err = Inspect(s.root)
	})
}

// apply resolves the LED (once, cached from then on) and, when one was
// found, calls fn against it. A board with none found is a silent no-op —
// gosd-xtcs's locked decision — the same as a nil boot.Deps.StatusLED.
func (s *Sysfs) apply(fn func(LED) error) error {
	s.resolve()
	if s.err != nil {
		return s.err
	}
	if !s.disc.Found {
		return nil
	}
	return fn(s.disc.LED)
}

package boot

import (
	"fmt"
	"runtime/debug"
	"time"
)

// PanicRebootDelay is how long a panic's stack trace stays on the console
// before the reboot it triggers, matching the fatal path's pause.
const PanicRebootDelay = 5 * time.Second

// PanicGuard is gosd-init's panic policy. A panic that escapes any
// goroutine ends the whole process, and PID 1 ending is a kernel panic
// ("Attempted to kill init!") — on an unattended appliance that means dead
// until someone physically power-cycles it. So every long-running goroutine
// gosd-init starts runs inside a guard, which turns a panic into a stack
// trace on the console plus the same sync/sleep/reboot the boot sequence's
// fatal path uses. The `panic=10` on every board's kernel command line is
// the independent second belt, covering panics in goroutines gosd-init
// doesn't own (a third-party library's own goroutines, say). See gosd-fkkr.
type PanicGuard struct {
	Rebooter Rebooter

	// Sleep pauses between the panic report and the reboot, so the trace
	// is readable on a serial console. Defaults to time.Sleep.
	Sleep func(time.Duration)

	// Log records the panic; a nil Log discards it (the reboot still
	// happens, since a lost log is no reason to leave PID 1 dead).
	Log func(format string, args ...any)
}

// Go runs fn in a new goroutine, guarded — see Guard.
func (g PanicGuard) Go(name string, fn func()) {
	go g.Guard(name, fn)
}

// Guard runs fn on the calling goroutine, converting a panic that escapes
// it into a logged stack trace and a reboot. name identifies the guarded
// work in that log line.
func (g PanicGuard) Guard(name string, fn func()) {
	defer g.recoverPanic(name)
	fn()
}

// Reboot flushes disks and the console, then restarts the machine, after
// logging reason and pausing PanicRebootDelay so the console keeps it
// readable. FlushConsole is the same gosd-fs34 guarantee the boot
// sequence's own fatal path uses — a stack trace worth reading deserves the
// same protection against being cut off mid-transmission.
func (g PanicGuard) Reboot(reason string) {
	g.log("fatal: %s; rebooting in %s", reason, PanicRebootDelay)
	if g.Rebooter == nil {
		return
	}
	g.Rebooter.Sync()
	g.Rebooter.FlushConsole()
	g.sleep(PanicRebootDelay)
	g.Rebooter.Reboot()
}

// recoverPanic is the deferred half of Guard: it must be called directly by
// a defer statement for recover() to see the panic.
func (g PanicGuard) recoverPanic(name string) {
	r := recover()
	if r == nil {
		return
	}
	g.Reboot(fmt.Sprintf("panic in %s: %v\n%s", name, r, debug.Stack()))
}

func (g PanicGuard) log(format string, args ...any) {
	if g.Log == nil {
		return
	}
	g.Log(format, args...)
}

func (g PanicGuard) sleep(d time.Duration) {
	if g.Sleep == nil {
		time.Sleep(d)
		return
	}
	g.Sleep(d)
}

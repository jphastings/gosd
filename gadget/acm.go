package gadget

// ACM is a CDC-ACM serial Function. Once its Gadget is applied, the kernel
// surfaces it at runtime as /dev/ttyGS0 — the tty's number comes from
// registration order across every ACM function on the system, not from the
// configfs instance name below.
type ACM struct{}

// Name implements Function. "usb0" is this gadget's only ACM instance;
// configfs requires the "<type>.<instance>" form regardless.
func (ACM) Name() string { return "acm.usb0" }

// Create implements Function: ACM needs no attribute files beyond the
// function directory itself, which Gadget.Apply already creates. The hook
// exists so future functions (e.g. ECM/RNDIS Ethernet) can write their own
// attributes here without changing Apply.
func (ACM) Create(writableFS, string) error { return nil }

package netup

import "sync"

// UpSet refcounts the shared network-up signal across independently
// managed interfaces (e.g. Ethernet and WiFi on a dual-interface board
// like pi-3b). netup and wifiup each run their own link/association state
// machine and previously called a single boolean MarkNetworkUp/
// ClearNetworkUp pair directly, so either medium going down clobbered the
// other's still-valid "up" state (bean gosd-akk4). Routing both through
// one UpSet fixes that: mark is invoked only on the empty-to-non-empty
// transition, and clear only on the non-empty-to-empty transition, so one
// interface's flap never clears a marker another, still-up interface
// still needs. The zero value is not usable; construct with NewUpSet.
type UpSet struct {
	mu    sync.Mutex
	up    map[string]struct{}
	mark  func() error
	clear func() error
}

// NewUpSet returns a ready-to-use UpSet backed by mark and clear.
func NewUpSet(mark, clear func() error) *UpSet {
	return &UpSet{up: make(map[string]struct{}), mark: mark, clear: clear}
}

// Up records iface as up, calling mark the first time the set becomes
// non-empty. A repeat Up for an iface already recorded up (e.g. a DHCP
// lease renewal) is a no-op — mark only needs to run on the transition.
func (s *UpSet) Up(iface string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, already := s.up[iface]; already {
		return nil
	}
	wasEmpty := len(s.up) == 0
	s.up[iface] = struct{}{}
	if wasEmpty {
		return s.mark()
	}
	return nil
}

// Down records iface as down, calling clear only once no interface in the
// set remains up. A Down for an iface not currently recorded up (never
// marked, or already cleared) is a no-op.
func (s *UpSet) Down(iface string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, present := s.up[iface]; !present {
		return nil
	}
	delete(s.up, iface)
	if len(s.up) == 0 {
		return s.clear()
	}
	return nil
}

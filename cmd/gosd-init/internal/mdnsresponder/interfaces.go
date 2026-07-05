package mdnsresponder

// Server is a running mDNS responder instance. The only operation Run
// itself needs is shutting one down before starting its replacement;
// everything else (which interfaces it's bound to, which names it
// answers) is fixed at construction time by NewServerFunc.
type Server interface {
	Close() error
}

// NewServerFunc starts a new Server answering for hostname+".local" on
// every interface that's up right now. Production wires this to this
// package's NewServer (server.go); tests supply a fake that never touches
// a real socket.
type NewServerFunc func(hostname string) (Server, error)

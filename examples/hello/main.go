// Command hello is a minimal example app used to exercise the gosd build
// pipeline end to end and to validate hardware bring-up. It prints a
// startup line, then serves an HTTP endpoint reporting hostname, uptime,
// and the request's remote address.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

var startTime = time.Now()

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	fmt.Printf("gosd hello, host=%s board=%s\n", hostname, os.Getenv("GOSD_BOARD"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s uptime=%s remote=%s\n", hostname, time.Since(startTime), r.RemoteAddr)
	})

	listener, err := net.Listen("tcp", ":80")
	if err != nil {
		listener, err = net.Listen("tcp", ":8080")
		if err != nil {
			fmt.Fprintf(os.Stderr, "gosd hello: failed to listen on :80 or :8080: %v\n", err)
			os.Exit(1)
		}
	}

	if err := http.Serve(listener, nil); err != nil {
		fmt.Fprintf(os.Stderr, "gosd hello: server stopped: %v\n", err)
		os.Exit(1)
	}
}

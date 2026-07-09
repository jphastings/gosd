// Command hello is a minimal example app used to exercise the gosd build
// pipeline end to end and to validate hardware bring-up. It prints a
// startup line, then serves an HTTP endpoint reporting hostname, uptime,
// the request's remote address, and — when the image has a GOSD-DATA
// partition — a boot counter persisted across reboots.
//
// It also demonstrates gosd.toml's [env] app environment variables (see
// docs/runtime.md's "App environment variables" section): an optional
// GREETING var, read with a plain os.Getenv, is included in the startup
// log and the HTTP response when set. Leave it unset and hello behaves
// exactly as it always has.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var startTime = time.Now()

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	boots := bumpBootCounter()
	greeting := greetingSuffix(os.Getenv("GREETING"))

	fmt.Printf("gosd hello, host=%s board=%s boots=%s%s\n", hostname, os.Getenv("GOSD_BOARD"), boots, greeting)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "host=%s uptime=%s remote=%s boots=%s%s\n", hostname, time.Since(startTime), r.RemoteAddr, boots, greeting)
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

// greetingSuffix turns the optional GREETING env var into a log/response
// suffix: empty (the var unset, per gosd.toml [env]'s "missing is fine"
// rule) yields "", so hello's output is byte-for-byte unchanged from
// before GREETING existed unless someone sets it.
func greetingSuffix(greeting string) string {
	if greeting == "" {
		return ""
	}
	return fmt.Sprintf(" greeting=%q", greeting)
}

// bumpBootCounter demonstrates GoSD's persistent storage: it increments a
// counter file on the GOSD-DATA partition every boot, using the
// write-to-temp + fsync + rename pattern docs/runtime.md recommends for
// FAT32's weak crash-safety. When GOSD_DATA is unset (the image was built
// with --data-size=0, or by an older gosd) there is no persistence, and the
// counter reports that instead of failing.
func bumpBootCounter() string {
	dataDir := os.Getenv("GOSD_DATA")
	if dataDir == "" {
		return "no-data-partition"
	}

	counterPath := filepath.Join(dataDir, "hello-boots")

	count := 0
	if raw, err := os.ReadFile(counterPath); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}
	count++

	if err := writeFileDurably(counterPath, []byte(strconv.Itoa(count)+"\n")); err != nil {
		fmt.Fprintf(os.Stderr, "gosd hello: persisting the boot counter failed: %v\n", err)
		return "write-failed"
	}
	return strconv.Itoa(count)
}

// writeFileDurably writes data to path so that a power cut leaves either the
// old contents or the new, never a torn mix: write a temp file, fsync it,
// then rename it over the real name.
func writeFileDurably(path string, data []byte) error {
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

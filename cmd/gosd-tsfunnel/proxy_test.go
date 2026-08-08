package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestReverseProxy_HeaderContract(t *testing.T) {
	var gotHost, gotProto string
	var gotForwardedFor string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotProto = r.Header.Get("X-Forwarded-Proto")
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("parsing backend URL: %v", err)
	}

	proxy := newReverseProxy(backendURL)
	front := httptest.NewServer(proxy)
	defer front.Close()

	req, err := http.NewRequest(http.MethodGet, front.URL, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	// The public Funnel hostname a visitor typed — must reach the backend
	// unchanged, not rewritten to the backend's own host:port.
	req.Host = "my-device.example.ts.net"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotHost != "my-device.example.ts.net" {
		t.Errorf("backend saw Host %q, want the inbound Funnel hostname preserved", gotHost)
	}
	if gotProto != "https" {
		t.Errorf("backend saw X-Forwarded-Proto %q, want %q (Funnel always terminates TLS on-node)", gotProto, "https")
	}
	if gotForwardedFor == "" {
		t.Error("backend saw no X-Forwarded-For at all, want the client address appended")
	}
	if !strings.Contains(gotForwardedFor, "127.0.0.1") {
		t.Errorf("X-Forwarded-For = %q, want it to contain the loopback client address", gotForwardedFor)
	}
}

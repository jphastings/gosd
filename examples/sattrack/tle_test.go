package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"
)

// The classic ISS TLE example (checksums valid), inclination 51.6416 deg.
const (
	issLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
	issLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
)

func TestFetchTLEParsesTheAPIResponse(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if !strings.HasSuffix(r.URL.Path, "/25544") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"@context":"https://www.w3.org/ns/hydra/context.jsonld","name":"ISS (ZARYA)","line1":"` + issLine1 + `","line2":"` + issLine2 + `"}`))
	}))
	defer srv.Close()

	got, err := fetchTLE(context.Background(), srv.Client(), srv.URL+"/", "25544")
	if err != nil {
		t.Fatalf("fetchTLE: %v", err)
	}
	if got.name != "ISS (ZARYA)" {
		t.Errorf("name = %q, want ISS (ZARYA)", got.name)
	}
	if gotUA != tleUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, tleUserAgent)
	}

	// The parsed satellite must actually propagate: subpoints of an
	// i=51.64 degree orbit stay within that latitude band.
	epoch := time.Date(2008, 9, 20, 12, 25, 40, 0, time.UTC)
	for i := 0; i < 90; i++ {
		p := subpoint(got.sgp4, epoch.Add(time.Duration(i)*time.Minute))
		if math.Abs(p.lat) > 52.5 || p.lon < -180 || p.lon >= 180 {
			t.Fatalf("subpoint %d = (%.2f, %.2f), outside the orbit's plausible band", i, p.lat, p.lon)
		}
	}
}

func TestFetchTLERejectsNonOKAndMissingLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/404") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"name":"NO LINES"}`))
	}))
	defer srv.Close()

	if _, err := fetchTLE(context.Background(), srv.Client(), srv.URL+"/", "404"); err == nil {
		t.Error("fetchTLE on a 404 succeeded, want an error")
	}
	if _, err := fetchTLE(context.Background(), srv.Client(), srv.URL+"/", "nolines"); err == nil {
		t.Error("fetchTLE on a response missing line1/line2 succeeded, want an error")
	}
}

func TestComputeFrameSamplesShareTheAbsoluteTenSecondGrid(t *testing.T) {
	s, err := fetchTLEFromLines(issLine1, issLine2)
	if err != nil {
		t.Fatalf("parsing the test TLE: %v", err)
	}

	now := time.Date(2008, 9, 20, 12, 25, 43, 0, time.UTC) // off-grid on purpose
	f1, err := computeFrame(s.sgp4, now)
	if err != nil {
		t.Fatalf("computeFrame: %v", err)
	}
	f2, err := computeFrame(s.sgp4, now.Add(time.Second))
	if err != nil {
		t.Fatalf("computeFrame: %v", err)
	}

	// Interior samples are grid-aligned and therefore shared between the
	// two ticks - that is what keeps per-tick repainting partial.
	shared := 0
	times1 := make(map[time.Time]bool, len(f1.past))
	for _, p := range f1.past[1 : len(f1.past)-1] {
		if p.t.Unix()%10 != 0 {
			t.Fatalf("interior past sample at %v is off the 10s grid", p.t)
		}
		times1[p.t] = true
	}
	for _, p := range f2.past[1 : len(f2.past)-1] {
		if times1[p.t] {
			shared++
		}
	}
	if shared < len(times1)-1 {
		t.Errorf("consecutive ticks share %d of %d interior samples; want all but at most one", shared, len(times1))
	}
}

// fetchTLEFromLines parses without the HTTP layer, fenced like fetchTLE.
func fetchTLEFromLines(l1, l2 string) (s sat, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("TLE parse panicked: %v", r)
		}
	}()
	return sat{name: "TEST", sgp4: satellite.TLEToSat(l1, l2, satellite.GravityWGS84)}, nil
}

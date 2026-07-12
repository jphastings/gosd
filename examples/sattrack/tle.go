package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"
)

const (
	tleBaseURL      = "https://tle.ivanstanojevic.me/api/tle/"
	tleUserAgent    = "gosd-sattrack-example"
	tleFetchTimeout = 15 * time.Second
	tleRefreshEvery = 6 * time.Hour
	tleBackoffCap   = 5 * time.Minute
)

// tleRecord is the api's response shape; its JSON-LD @context noise is
// ignored.
type tleRecord struct {
	Name  string `json:"name"`
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
}

// sat pairs a parsed, propagatable satellite with its display name.
type sat struct {
	name string
	sgp4 satellite.Satellite
}

// fetchTLE gets and parses one TLE. TLEToSat panics on malformed element
// sets, so parsing is fenced with a recover and reported as an ordinary
// error.
func fetchTLE(ctx context.Context, client *http.Client, baseURL, noradID string) (s sat, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parsing the TLE for %s failed: %v", noradID, r)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, tleFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+noradID, nil)
	if err != nil {
		return sat{}, err
	}
	req.Header.Set("User-Agent", tleUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return sat{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return sat{}, fmt.Errorf("GET %s%s: %s", baseURL, noradID, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return sat{}, err
	}
	var rec tleRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return sat{}, fmt.Errorf("decoding the TLE response for %s: %w", noradID, err)
	}
	if rec.Line1 == "" || rec.Line2 == "" {
		return sat{}, fmt.Errorf("TLE response for %s is missing line1/line2", noradID)
	}

	return sat{name: rec.Name, sgp4: satellite.TLEToSat(rec.Line1, rec.Line2, satellite.GravityWGS84)}, nil
}

// tleSource fetches a satellite's TLE at startup (retrying forever with
// exponential backoff - the network may come up well after gosd-init
// starts the app) and refreshes it every 6h, keeping the old element set
// when a refresh fails.
type tleSource struct {
	client  *http.Client
	baseURL string
	noradID string
	updates chan sat
	kicks   chan struct{}
}

func newTLESource(noradID string) *tleSource {
	return &tleSource{
		client:  &http.Client{},
		baseURL: tleBaseURL,
		noradID: noradID,
		updates: make(chan sat, 1),
		kicks:   make(chan struct{}, 1),
	}
}

// kick asks run for a fresh fetch ahead of the 6h schedule (e.g. after the
// current element set turned out to be unpropagatable). Never blocks.
func (s *tleSource) kick() {
	select {
	case s.kicks <- struct{}{}:
	default:
	}
}

// run blocks, delivering the first TLE (and every 6-hourly refresh that
// yields one) on s.updates.
func (s *tleSource) run(ctx context.Context) {
	backoff := time.Second
	for {
		got, err := fetchTLE(ctx, s.client, s.baseURL, s.noradID)
		if err == nil {
			log.Printf("sattrack: TLE for %s (%q) fetched", s.noradID, got.name)
			select {
			case s.updates <- got:
			case <-ctx.Done():
				return
			}
			backoff = time.Second
			select {
			case <-time.After(tleRefreshEvery):
			case <-s.kicks:
			case <-ctx.Done():
				return
			}
			continue
		}

		if ctx.Err() != nil {
			return
		}
		log.Printf("sattrack: fetching the TLE for %s failed (%v); retrying in %s", s.noradID, err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = min(backoff*2, tleBackoffCap)
	}
}

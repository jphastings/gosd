// Package faultdrop defines the file an app's fault.Fatal call leaves in
// /run for gosd-init to pick up, and reads it back (bean gosd-aa1p, epic
// gosd-47z3).
//
// An app must never write LAST_FATAL_ERROR.md itself: gosd-init owns the
// boot partition's mount state, and an app that remounted it read-write
// would race gosd-init's own remounts and leave the card writable under a
// live app. So a declared fatal travels as a file drop instead — no
// listener, no protocol, consistent with gosd-init having no interactive
// surface.
//
// # File format
//
// [Path] is /run/gosd/fault.json — /run is tmpfs, already mounted by
// gosd-init's early mounts. The file is a single JSON object holding the
// human and technical halves of one [faultreport.Report], with every field
// optional:
//
//	{
//	  "code": "NO-API-KEY",
//	  "doing": "fetching today's forecast",
//	  "problem": "the weather service rejected our API key",
//	  "fix": "add WEATHER_API_KEY to gosd.toml on this card",
//	  "detail": "get \"https://api.example\": 401 unauthorized"
//	}
//
// Named optional fields rather than a positional or versioned encoding,
// because the two ends of this handoff are NOT necessarily the same
// release: the app compiles fault from whatever gosd version its own go.mod
// pins, while gosd-init is built by the gosd CLI doing the build. An older
// app talking to a newer gosd-init (or the reverse) then loses at worst a
// field, never the whole report.
//
// Mode 0600 — the technical detail can carry anything the app's error chain
// carried, secrets included, and nothing else on the device has a reason to
// read it. The writer writes to Path+".tmp" and renames, the same
// discipline as [github.com/jphastings/gosd/internal/secretreg] and every
// other file in this codebase that must never be read half-written, so
// gosd-init only ever sees a complete file. There is no fsync in that
// sequence and there should not be: /run is RAM, so there is no durability
// to buy, only the atomicity rename already provides.
//
// # What a reader must assume
//
// [Parse] drops the file wholesale rather than partially trust it when it
// is empty, larger than [MaxBytes], not this shape, or carries no content
// at all — the gosd-6cf2 self-heal lesson: a state file that can wedge its
// reader on one bad byte is worse than one that is occasionally ignored.
// [Take] removes the file whether or not it parsed, so a report is
// delivered exactly once and an unparseable one can't haunt every
// subsequent exit.
package faultdrop

import (
	"encoding/json"
	"os"
	"unicode/utf8"

	"github.com/jphastings/gosd/internal/faultreport"
)

// Dir is the tmpfs directory the drop file lives in, shared with the rest
// of gosd-init's runtime state.
const Dir = "/run/gosd"

// Path is where fault.Fatal writes its report and gosd-init picks it up.
const Path = Dir + "/fault.json"

// MaxBytes bounds the encoded file, both for the writer (which trims to
// fit) and the reader (which refuses anything larger). /run is tmpfs —
// memory shared with everything else on a board that may have only 512MB —
// and an app can put an arbitrarily large error chain in Detail, so this is
// the difference between a crash report and a memory-pressure incident.
const MaxBytes = 64 * 1024

// maxHumanBytes bounds each of the report's human-facing fields. They are
// meant to be a sentence each; anything beyond this is a bug in the calling
// app, and trimming it keeps the technical detail's budget predictable no
// matter how JSON escaping expands the rest.
const maxHumanBytes = 1024

// truncationMarker is appended to any field this package had to trim, so a
// reader of the finished report can tell a short detail from a shortened
// one.
const truncationMarker = "\n… (truncated by gosd: the report was too large to hand over whole)"

// ExitCode is the status fault.Fatal exits the app with. It is not how
// gosd-init decides a fault was declared — the drop file is, since a fatal
// can coincide with a signal that replaces the status entirely — but it
// keeps the supervisor's console line ("exited with status 70")
// distinguishable from an ordinary failure at a glance. 70 is sysexits.h's
// EX_SOFTWARE.
const ExitCode = 70

// dropped is the file's exact shape. It mirrors faultreport.Report field
// for field; keeping a separate type is what pins the JSON names as the
// on-tmpfs contract rather than letting a rename in the renderer silently
// change the wire format between two releases.
type dropped struct {
	Code    string `json:"code,omitempty"`
	Doing   string `json:"doing,omitempty"`
	Problem string `json:"problem,omitempty"`
	Fix     string `json:"fix,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Marshal encodes r as the drop file's bytes, trimming it to fit MaxBytes
// if it has to: the human fields to a sentence's worth each, and then the
// technical detail by halves until the encoded object fits. A trimmed
// report is far better than none — the detail's head is where an error
// chain says what failed — and every trim is marked in the text so nobody
// reads a truncated stack trace as a complete one.
func Marshal(r faultreport.Report) ([]byte, error) {
	d := dropped{
		Code:    trim(r.Code, maxHumanBytes),
		Doing:   trim(r.Doing, maxHumanBytes),
		Problem: trim(r.Problem, maxHumanBytes),
		Fix:     trim(r.Fix, maxHumanBytes),
	}

	// Each attempt trims the ORIGINAL detail rather than the previous
	// attempt's output, so a report trimmed four times still carries one
	// truncation marker instead of four. Halving, rather than computing a
	// target length, is deliberate: JSON escaping can turn one byte into
	// six (a control character becomes a six-character escape), so no
	// arithmetic on the unescaped length gets this right in a single pass.
	for limit := len(r.Detail); ; limit /= 2 {
		d.Detail = trim(r.Detail, limit)
		data, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		if len(data) <= MaxBytes || limit == 0 {
			return data, nil
		}
	}
}

// Parse turns the drop file's bytes into a report, reporting false when
// there is nothing trustworthy in them: empty, oversized, not this shape,
// or an object with every field blank (a report that says nothing is
// indistinguishable from no report, and writing "Nothing was captured" onto
// a card teaches its owner nothing).
func Parse(data []byte) (faultreport.Report, bool) {
	if len(data) == 0 || len(data) > MaxBytes {
		return faultreport.Report{}, false
	}

	var d dropped
	if err := json.Unmarshal(data, &d); err != nil {
		return faultreport.Report{}, false
	}
	if d == (dropped{}) {
		return faultreport.Report{}, false
	}

	return faultreport.Report{
		Code:    d.Code,
		Doing:   d.Doing,
		Problem: d.Problem,
		Fix:     d.Fix,
		Detail:  d.Detail,
	}, true
}

// Take reads the drop file at path, removes it, and returns the report it
// held. The file is removed whether or not it parsed, and so is any .tmp
// left behind by an app that died mid-write, so that one bad drop can't be
// re-read (or re-reported) on every subsequent exit. A missing file — by
// far the common case, since most app exits declare nothing — is not an
// error: it simply reports false.
//
// The size is checked before the read, not after: [Marshal] keeps a report
// under [MaxBytes], but nothing stops an app writing this path itself, and
// gosd-init is PID 1 on a board with as little as 512MB. Reading an
// arbitrarily large file into memory just to reject it is the failure this
// avoids.
func Take(path string) (faultreport.Report, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return faultreport.Report{}, false
	}
	if info.Size() > MaxBytes {
		remove(path)
		return faultreport.Report{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return faultreport.Report{}, false
	}
	remove(path)
	return Parse(data)
}

// remove deletes the drop file and any .tmp beside it. Both are
// best-effort: a report that couldn't be deleted would be re-read on the
// next exit, which is a worse report rather than a failed boot.
func remove(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + ".tmp")
}

// trim shortens s to at most limit bytes, cutting on a rune boundary and
// marking the cut. A limit small enough that the marker alone wouldn't fit
// is honoured anyway: the caller is trimming to fit a hard bound, and an
// unmarked short string beats overshooting it.
func trim(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit] + truncationMarker
}

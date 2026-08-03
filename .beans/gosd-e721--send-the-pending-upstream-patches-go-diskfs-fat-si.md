---
# gosd-e721
title: Send the pending upstream patches (go-diskfs FAT sizing + label trim, pion/mdns leak)
status: todo
type: task
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-03T18:30:55Z
---

Three upstream fixes were written up during the review sweep but not
sent, per the no-third-party-PRs rule — JP decides and sends:

- go-diskfs sectors-per-FAT under-computation: one-line numerator fix
  with full derivation, proof sweep, and two caveats for the sender —
  recorded in bean gosd-e3e3's Summary of Changes.
- go-diskfs volume-label per-field 8.3 trim (interior byte-7 space
  loss): sketched patch (trim the concatenated 11 raw bytes once for
  the volume-label entry) in bean gosd-f83b's Summary.
- pion/mdns v2.1.0 leaks its two internal unicast sockets on Server()'s
  early error returns: exact conn.go line references and suggested
  patch in bean gosd-o6tp's Summary.

GoSD carries local mitigations for all three, so none is urgent; sending
them retires the mitigations' upstream halves eventually.

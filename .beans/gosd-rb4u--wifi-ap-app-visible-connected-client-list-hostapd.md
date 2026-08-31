---
# gosd-rb4u
title: 'WiFi AP: app-visible connected-client list (hostapd ctrl_interface)'
status: todo
type: task
created_at: 2026-08-31T06:38:18Z
updated_at: 2026-08-31T06:38:18Z
---

Follow-up to WiFi AP epic gosd-qfbk, which deliberately shipped v1 with no
app-visible list of connected clients (epic decision 12, JP 2026-08-31 —
"start without client visibility, but know that it'll likely be a follow-up
piece of work"). Not blocking the epic; open this once v1 has shipped and
there's a concrete use case driving the shape of the API.

## Why it wasn't in v1

An app plausibly wants to know when a phone joins its AP — a pairing or
provisioning flow is the obvious case. Getting that requires hostapd's
`ctrl_interface`, a local unix-domain socket hostapd exposes for querying
station state (`STA-CONNECTED`/`STA-DISCONNECTED` events, `all_sta`).

That is NOT network-reachable, so it doesn't reopen the network-listener
question the epic amended — but it IS a deliberate widening of that
amendment's deliberately narrow scope ("v1 does not enable hostapd's own
`ctrl_interface` ... so the practical new listener surface this amendment
authorizes is the DHCP server alone"). It also gives gosd-init a genuine
IPC channel to a supervised child for the first time, which is a real
design step rather than a small addition. Both reasons to do it
deliberately, with its own decision, rather than folding it into v1.

## What this bean would need to settle

- Enable `ctrl_interface` in the generated `hostapd.conf` (path under
  `/run/gosd/wifiap/`, 0600, root-only).
- Whether the app reads station state directly, or gosd-init proxies it —
  gosd-init supervises hostapd, so a direct app→hostapd socket read means
  two readers on one control socket.
- The app-facing API shape on the public `wifiap` package (epic decision
  11): a point-in-time `Clients()` list, an event stream, or both.
- Update the CLAUDE.md amendment's final clause (the one that currently
  says v1 authorizes the DHCP server alone) to name the control socket.
- Whether a DHCP-lease-table view (already available in-process from the
  `dhcpserver` package, bean gosd-3ye3, with no hostapd involvement at all)
  covers enough of the real use case to make `ctrl_interface` unnecessary —
  worth checking FIRST, since it needs no new socket, no amendment change,
  and no IPC design. A lease table tells you what got an IP; hostapd tells
  you what associated. For "has a phone connected yet?", the lease may well
  be the more useful signal anyway.

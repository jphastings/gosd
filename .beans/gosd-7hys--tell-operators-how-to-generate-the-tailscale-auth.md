---
# gosd-7hys
title: Tell operators how to generate the Tailscale auth key
status: completed
type: task
priority: normal
created_at: 2026-11-01T18:51:21Z
updated_at: 2026-11-01T18:51:21Z
---

The card's `config/ingress/tailscale-funnel/authkey.explain.md` says to use "a reusable, tagged auth key, made in the Tailscale admin console" but never says where to go or what to click.

JP (2026-11-01): open https://console.tailscale.com/admin/settings/keys and click "Generate auth key". **Ephemeral must be off.** The key does not need to be reusable. The tag stays, because tagging is what turns off the node's key expiry.

This amends epic gosd-65uy's locked decision 4 ("TAGGED, REUSABLE") to "tagged, not ephemeral; reusable optional". The ingress guide and the tsfunnel doc comment repeat that decision, so they change with it.

- [x] Rewrite authkey.explain.md with the steps
- [x] Update the ingress guide's runbook, cross-reference and settings table
- [x] Update the tsfunnel.Config doc comment
- [x] Amend gosd-65uy decision 4
- [x] Change file

## Summary of Changes

The card's authkey explain now gives the steps: the keys page URL, "Generate auth key", Ephemeral off, the Funnel tag, and a note that reusable isn't needed. The ingress guide's runbook, its cross-reference and its settings table now say "tagged, non-ephemeral" instead of "tagged, reusable", and so does the `tsfunnel.Config` doc comment. gosd-65uy decision 4 carries a dated amendment.

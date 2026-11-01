---
gosd: patch
---

#### The Tailscale auth key's explain file says how to make one

`config/ingress/tailscale-funnel/authkey.explain.md` on the card now walks
through generating the key: open the Tailscale admin console's keys page,
click "Generate auth key", leave Ephemeral off and add your Funnel tag. The
key no longer needs to be reusable. A fresh key with the same tag
re-registers a device just as well.

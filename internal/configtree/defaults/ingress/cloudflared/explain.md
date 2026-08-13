# Cloudflare Tunnel

This folder lets the app on this device be reached from the internet
using a free Cloudflare Tunnel, at a web address of your choosing.

To use it, fill in all three files in this folder:

- `token` — the tunnel's token from Cloudflare, treated like a password.
- `hostname` — the public web address people will use to reach it.
- `port` — the port number the app on this device listens on.

Each file has its own `explain.md`-style notes below, but if you'd
rather jump straight in: create a tunnel in the Cloudflare dashboard
(or with `cloudflared tunnel token <tunnel-name>`), and fill in each
file accordingly.

The tunnel doesn't start until the `token` file has been filled in —
until then, this folder has no effect at all.

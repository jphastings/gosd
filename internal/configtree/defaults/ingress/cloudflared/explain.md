# Cloudflare Tunnel

This folder lets the app on this device be reached from the internet
using a free Cloudflare Tunnel, at a web address of your choosing.

To use it, fill in all three files in this folder:

- `token` — the tunnel's token from Cloudflare, treated like a password.
- `hostname` — the public web address people will use to reach it.
- `port` — the port number the app on this device listens on.

Each file has its own notes explaining what to type into it. What follows
here is how to get a token in the first place, if you don't have one yet.

## What you need

- A free Cloudflare account: https://dash.cloudflare.com
- A domain already set up on that account, with permission to add DNS
  records and tunnels to it.
- The `cloudflared` command-line tool installed on your own computer (not
  this device) — on a Mac with Homebrew, `brew install cloudflared`; for
  other systems, see Cloudflare's own install instructions.

## Getting a token

Run these on your own computer, not on this device:

1. Log in: `cloudflared tunnel login`
2. Create your tunnel, if you haven't already — pick any name you like:
   `cloudflared tunnel create <the-name-you-pick>`
3. Print the token this folder needs:
   `cloudflared tunnel token <the-name-you-pick>`
4. Route traffic from your domain to the tunnel:
   `cloudflared tunnel route dns <your-domain.com> <the-name-you-pick>`

Step 3 prints one long string — paste that into the `token` file in this
folder. Use the same domain name (or a subdomain of it) you routed in step
4 as this folder's `hostname` value.

The tunnel doesn't start until the `token` file has been filled in —
until then, this folder has no effect at all.

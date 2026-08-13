# Tailscale Funnel

This folder lets the app on this device be reached from the internet
using Tailscale Funnel, at a public address that looks like:

    https://my-device.your-tailnet.ts.net

To use it, you'll need a Tailscale account and, the first time only, an
auth key from the Tailscale admin console (see `authkey`). You'll also
need to say which port the app listens on (see `port`), and optionally
a public name and internet-facing port (see `hostname` and
`funnel_port`).

If you don't want this device reachable through Tailscale, leave every
file in this folder empty.

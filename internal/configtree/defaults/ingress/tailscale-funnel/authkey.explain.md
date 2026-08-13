# Tailscale auth key

This is a reusable, tagged auth key, made in the Tailscale admin
console, that lets this device register itself with your Tailscale
network without you having to click a login link.

It's only needed the first time this device registers. After that has
happened successfully, you can safely empty this file again — the
device remembers its registration on its own.

Paste the key in with no extra spaces or line breaks, for example:

    tskey-auth-xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

Treat it like a password. Leave this file empty if you aren't setting
up Tailscale Funnel, or once this device is already registered.

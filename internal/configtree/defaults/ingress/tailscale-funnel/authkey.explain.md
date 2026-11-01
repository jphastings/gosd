# Tailscale auth key

This is an auth key that lets this device join your Tailscale network
on its own, without you having to click a login link.

To make one:

1. Go to https://console.tailscale.com/admin/settings/keys and click
   "Generate auth key".
2. Leave "Ephemeral" switched off. An ephemeral device is removed from
   your network soon after it goes offline, taking its public address
   with it.
3. Add the tag your Tailscale access controls allow to use Funnel (for
   example `tag:gosd-device`). Without a tag, this device's login
   expires after a few months and it drops off your network.
4. Generate the key and copy it. It doesn't need to be reusable.

Paste the key in with no extra spaces or line breaks, for example:

    tskey-auth-xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

It's only needed the first time this device registers. After that has
happened successfully, you can safely empty this file again — the
device remembers its registration on its own.

Treat it like a password. Leave this file empty if you aren't setting
up Tailscale Funnel, or once this device is already registered.

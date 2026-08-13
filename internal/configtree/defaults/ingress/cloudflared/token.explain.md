# Cloudflare Tunnel token

This is the token for your Cloudflare Tunnel — a long piece of text
that proves this device is allowed to use the tunnel. You can copy it
from the Cloudflare dashboard when you create the tunnel, or by running:

    cloudflared tunnel token <tunnel-name>

Paste it into this file with no extra spaces or line breaks before or
after it. Treat it like a password — anyone who has it can use your
tunnel.

If you open this file and it looks blank, that's expected: it's a
plain text file that has deliberately been made long enough to hold a
token of this size, so there's plenty of room for you to paste into.

Leave it empty (as shipped) to leave the tunnel switched off — nothing
in this folder takes effect until a token is filled in.

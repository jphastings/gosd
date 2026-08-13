# Tailscale Funnel internet-facing port

This decides which port people use when they type in the public
address for this device. Tailscale Funnel only allows a choice of
three: `443`, `8443`, or `10000`.

Type one of those three numbers, for example:

    443

Leave this file empty to use the default, `443` — the ordinary port
used for secure web addresses, meaning people won't need to type a
port number at all.

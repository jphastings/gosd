---
gosd: patch
---

#### gosd-init steps the clock forward to the image's build time at boot

A board with no working RTC used to run at the Unix epoch until SNTP answered, so every TLS handshake failed in that window. `gosd-init` now raises an earlier clock to the image's build time before anything else starts, so cloudflared, tailscale-funnel and your app can validate certificates before NTP lands. The clock is only ever moved forward, the RTC is not written, and `/run/gosd/time-synced` still appears only after a real sync. Until then a certificate that expired after the build still validates; see the runtime contract's clock section.

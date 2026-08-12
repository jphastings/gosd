---
# gosd-q4v5
title: A NUL byte in a gosd.toml [env] value stops /app starting, forever, on every boot
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
---

**Severity: Medium.** Permanent denial of service for the one thing the
appliance exists to do, from one card edit. gosd-init itself stays healthy,
so the board looks alive while doing nothing.

## Verified

`internal/gosdtoml/config.go:263-298` (`coerceEnv`) validates TOML **type**
only:

```go
case string:
    env[key] = value
```

No content check. A TOML basic string encodes arbitrary bytes via a
backslash-u escape, so a value written as `bar

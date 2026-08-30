---
artifacts: minor
---

#### Turing RK1 kernel and U-Boot are now published in artifacts releases

The Turing RK1's compiled kernel and mainline U-Boot (idbloader + FIT with
BL31, rkbin DDR-init blobs) are now attached to artifacts releases,
alongside every other board's. The board itself isn't buildable via `gosd
build` yet — that's a separate, later step — this just gets its compiled
output into a real release for the first time.

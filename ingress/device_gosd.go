//go:build gosd

package ingress

import "github.com/jphastings/gosd/internal/ingressurl"

// runDir is where each ingress agent records its public URL: tmpfs under
// /run/gosd that gosd-init mounts before it starts an app. The gate is the
// `gosd` build tag rather than a filesystem probe, for the reason given in
// wifi/device_gosd.go.
const runDir = ingressurl.Dir

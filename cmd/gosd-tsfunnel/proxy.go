package main

import (
	"net/http/httputil"
	"net/url"
)

// newReverseProxy builds the proxy that carries every Funnel request to
// backend. Three header decisions are locked by the epic (gosd-65uy): the
// inbound Host header (the public *.ts.net name) passes through unchanged
// rather than being rewritten to the backend's own host, so an app reading
// r.Host sees the name its visitor actually typed; X-Forwarded-For gets
// ProxyRequest's own default handling; and X-Forwarded-Proto is set to
// "https" explicitly rather than inferred from the inbound connection —
// Funnel always terminates TLS on-node before gosd-tsfunnel ever sees a
// request, but the connection this process accepts from tsnet's Funnel
// listener is already-decrypted plain HTTP, so SetXForwarded's own
// TLS-based inference would report "http" instead.
func newReverseProxy(backend *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(backend)
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Proto", "https")
		},
	}
}

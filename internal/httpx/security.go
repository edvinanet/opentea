// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import "net/http"

// IsSecure reports whether r is effectively an HTTPS request: either this
// process terminated TLS itself (r.TLS != nil), or trustProxyHeaders is set
// and r carries X-Forwarded-Proto: https -- the usual signal a
// TLS-terminating reverse proxy sets on the plaintext connection it forwards
// to this process. See config.Config.TrustProxyHeaders's doc comment for
// why that header is only trusted when explicitly opted into: it's exactly
// as spoofable as any other client-supplied header otherwise.
//
// Used for anything that should reflect "is this session actually
// protected by TLS end-to-end" -- e.g. the session cookie's Secure flag and
// whether to send Strict-Transport-Security.
func IsSecure(r *http.Request, trustProxyHeaders bool) bool {
	if r.TLS != nil {
		return true
	}
	return trustProxyHeaders && r.Header.Get("X-Forwarded-Proto") == "https"
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestIsSecure(t *testing.T) {
	cases := []struct {
		name              string
		tls               bool
		forwardedProto    string
		trustProxyHeaders bool
		want              bool
	}{
		{"plain HTTP, no proxy trust", false, "", false, false},
		{"real TLS connection", true, "", false, true},
		{"real TLS connection, proxy trust irrelevant", true, "", true, true},
		{"X-Forwarded-Proto https but not trusted", false, "https", false, false},
		{"X-Forwarded-Proto https and trusted", false, "https", true, true},
		{"X-Forwarded-Proto http and trusted", false, "http", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if c.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", c.forwardedProto)
			}
			if got := IsSecure(req, c.trustProxyHeaders); got != c.want {
				t.Errorf("IsSecure(tls=%v, X-Forwarded-Proto=%q, trustProxyHeaders=%v) = %v, want %v",
					c.tls, c.forwardedProto, c.trustProxyHeaders, got, c.want)
			}
		})
	}
}

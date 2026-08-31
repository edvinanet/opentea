// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
)

// sessionCookieName deliberately differs from opentea's own "opentea_session"
// (internal/authn.SessionCookieName) -- both processes may run on the same
// host/domain during development, and sharing a cookie name/path would
// let one app's session leak into (or collide with) the other's.
const sessionCookieName = "openteapublisher_session"

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.sessionUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, pageData{})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	// Checked before touching the form body or calling VerifyLogin at all,
	// so a throttled client can't burn CPU on bcrypt (or anything else) by
	// retrying -- see httpx.LoginLimiter's doc comment.
	if !s.loginLimiter.Allow(httpx.ClientIP(r, s.cfg.TrustProxyHeaders)) {
		s.renderLogin(w, pageData{Error: "too many login attempts, please wait a moment and try again"})
		return
	}

	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		s.renderLogin(w, pageData{Error: "invalid form submission"})
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")

	staff, err := s.repo.VerifyLogin(r.Context(), username, password)
	if errors.Is(err, ErrInvalidCredentials) {
		s.renderLogin(w, pageData{Error: "invalid username or password"})
		return
	}
	if err != nil {
		s.renderLogin(w, pageData{Error: "internal error, please try again"})
		return
	}

	token, expiresAt, err := s.repo.CreateSession(r.Context(), staff.UUID)
	if err != nil {
		s.renderLogin(w, pageData{Error: "internal error, please try again"})
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure/HttpOnly/SameSite are all set below; gosec's G124 flags the literal without checking its fields
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   httpx.IsSecure(r, s.cfg.TrustProxyHeaders),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.repo.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure/HttpOnly/SameSite are all set below; gosec's G124 flags the literal without checking its fields
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   httpx.IsSecure(r, s.cfg.TrustProxyHeaders), // match the Secure flag used when this cookie was originally set
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

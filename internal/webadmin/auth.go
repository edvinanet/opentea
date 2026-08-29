// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := authn.SessionUser(r.Context(), r, s.repo); ok {
		http.Redirect(w, r, "/admin/ui/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, pageData{})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	// Checked before touching the form body or calling VerifyLogin at all,
	// so a throttled client can't burn CPU on bcrypt (or anything else) by
	// retrying -- see loginLimiter's doc comment.
	if !s.loginLimiter.allow(clientIP(r, s.cfg.TrustProxyHeaders)) {
		s.renderLogin(w, pageData{Error: "too many login attempts, please wait a moment and try again"})
		return
	}

	if err := r.ParseForm(); err != nil {
		// A body rejected by limitBody's MaxBytesReader surfaces here as a
		// *http.MaxBytesError -- worth a real 4xx (not the generic re-render
		// below, which returns 200) since it's a distinct, controlled
		// rejection rather than a malformed submission.
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

	user, err := s.repo.VerifyLogin(r.Context(), username, password)
	if errors.Is(err, repo.ErrInvalidCredentials) {
		s.renderLogin(w, pageData{Error: "invalid username or password"})
		return
	}
	if err != nil {
		s.renderLogin(w, pageData{Error: "internal error, please try again"})
		return
	}

	token, expiresAt, err := s.repo.CreateSession(r.Context(), user.UUID, authn.SessionTTL)
	if err != nil {
		s.renderLogin(w, pageData{Error: "internal error, please try again"})
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure/HttpOnly/SameSite are all set below; gosec's G124 flags the literal without checking its fields
		Name:     authn.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   httpx.IsSecure(r, s.cfg.TrustProxyHeaders),
	})
	http.Redirect(w, r, "/admin/ui/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(authn.SessionCookieName); err == nil {
		_ = s.repo.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure/HttpOnly/SameSite are all set below; gosec's G124 flags the literal without checking its fields
		Name:     authn.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   httpx.IsSecure(r, s.cfg.TrustProxyHeaders), // match the Secure flag used when this cookie was originally set
	})
	http.Redirect(w, r, "/admin/ui/login", http.StatusSeeOther)
}

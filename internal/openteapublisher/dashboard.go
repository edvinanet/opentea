// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import "net/http"

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, staff Staff) {
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "dashboard", pageData{Staff: staff, Targets: targets})
}

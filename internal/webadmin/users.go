package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) usersPage(w http.ResponseWriter, r *http.Request, user model.User) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "users", pageData{User: user, Users: users})
}

func (s *Server) createUserForm(w http.ResponseWriter, r *http.Request, user model.User) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/ui/users", http.StatusSeeOther)
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	role := r.PostFormValue("role")

	if username == "" || password == "" || (role != model.RoleAdmin && role != model.RoleConsumer) {
		s.rerenderUsersWithError(w, r, user, "username, password, and a valid role are required")
		return
	}

	_, err := s.repo.CreateUser(r.Context(), username, password, role)
	if errors.Is(err, repo.ErrUsernameTaken) {
		s.rerenderUsersWithError(w, r, user, "username already taken")
		return
	}
	if errors.Is(err, repo.ErrPasswordTooShort) {
		s.rerenderUsersWithError(w, r, user, err.Error())
		return
	}
	if err != nil {
		s.rerenderUsersWithError(w, r, user, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/admin/ui/users", http.StatusSeeOther)
}

func (s *Server) deleteUserForm(w http.ResponseWriter, r *http.Request, user model.User) {
	uuid := r.PathValue("uuid")
	err := s.repo.DeleteUser(r.Context(), uuid)
	switch {
	case err == nil, errors.Is(err, repo.ErrNotFound):
		http.Redirect(w, r, "/admin/ui/users", http.StatusSeeOther)
	case errors.Is(err, repo.ErrLastAdmin):
		s.rerenderUsersWithError(w, r, user, "cannot delete the last admin user")
	default:
		s.rerenderUsersWithError(w, r, user, "internal error, please try again")
	}
}

func (s *Server) rerenderUsersWithError(w http.ResponseWriter, r *http.Request, user model.User, message string) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "users", pageData{User: user, Users: users, Error: message})
}

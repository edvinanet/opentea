package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// componentRefView resolves a tea.ComponentRef (which only carries UUIDs)
// to a display-friendly name, for rendering a product release's linked
// components. ReleaseUUID is empty for an unpinned reference. Deleted is set
// when the linked component no longer exists (a stale reference), so the
// template can skip linking to a page that would just 404.
type componentRefView struct {
	UUID        string
	Name        string
	ReleaseUUID string
	Deleted     bool
}

func (s *Server) productReleaseDetailPage(w http.ResponseWriter, r *http.Request, user model.User) {
	uuid := r.PathValue("uuid")

	release, err := s.repo.GetProductRelease(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), repo.OwnerProductRelease, uuid)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	components := make([]componentRefView, 0, len(release.Components))
	for _, ref := range release.Components {
		component, err := s.repo.GetComponent(r.Context(), ref.UUID)
		// A linked component can be deleted after this release's link row
		// was written (or in the narrow window between listing the release
		// and resolving this ref) -- that's a stale reference on this one
		// row, not a reason to fail the whole page, since the release
		// itself and everything else on it is still perfectly valid.
		if errors.Is(err, repo.ErrNotFound) {
			components = append(components, componentRefView{UUID: ref.UUID, Name: "(deleted component)", Deleted: true})
			continue
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		view := componentRefView{UUID: ref.UUID, Name: component.Name}
		if ref.Release != nil {
			view.ReleaseUUID = *ref.Release
		}
		components = append(components, view)
	}

	collections, err := s.repo.ListCollections(r.Context(), uuid, "desc", nil, listPageLimit, repo.BelongsToProductRelease)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.renderAuthenticated(w, "productRelease", pageData{
		User: user, ProductRelease: release, ProductReleaseCLE: cle,
		ProductReleaseComponents: components, Collections: collections,
	})
}

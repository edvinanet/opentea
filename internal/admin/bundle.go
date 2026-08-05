package admin

import (
	"archive/zip"
	"errors"
	"fmt"
	"net/http"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

const maxImportBundleSize = 1 << 30 // 1 GiB cap, matching the upload endpoints

// exportProduct streams a product's complete bundle zip -- see
// docs/bundle-format.md. Admin-only, per the user's explicit requirement
// that import/export requires admin authorization (not just consumer).
func (s *Server) exportProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	if _, err := s.repo.GetProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="product-%s.zip"`, uuid))
	if err := bundle.Export(r.Context(), s.repo, s.storage, uuid, w); err != nil {
		// Headers are already sent at this point, so we can't fall back to
		// a JSON error response -- just log server-side via InternalError's
		// existing logging path won't re-send headers either. Best effort:
		// the client will see a truncated/invalid zip, which is at least
		// detectable.
		httpx.InternalError(w, r, err)
		return
	}
}

// importProduct accepts a multipart-uploaded bundle zip (field "bundle") and
// applies it idempotently. Admin-only, entirely separate from /tea/v1 and
// any future publisher API.
func (s *Server) importProduct(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBundleSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("bundle")
	if err != nil {
		httpx.BadRequest(w, `missing "bundle" form field: `+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	// multipart.File always implements io.ReaderAt (backed by an on-disk
	// temp file once ParseMultipartForm's 32MiB memory threshold above is
	// exceeded, an in-memory buffer below it) -- reading zr's entries
	// through file directly, instead of first buffering the whole upload
	// into a []byte, keeps a large bundle import from holding a second
	// full copy (up to maxImportBundleSize) in memory on top of whatever
	// ParseMultipartForm itself already retains.
	zr, err := zip.NewReader(file, header.Size)
	if err != nil {
		httpx.BadRequest(w, "bundle is not a valid zip file: "+err.Error())
		return
	}

	result, err := bundle.Import(r.Context(), s.repo, s.storage, s.cfg.RootURL, zr)
	if err != nil {
		httpx.BadRequest(w, "import failed: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

const maxUploadBody = 1 << 30 // 1 GiB cap per uploaded file

// receiveFile extracts the "file" multipart field, stores it via the blob
// store, records blob bookkeeping (for /files/{sha256}'s Content-Type), and
// returns the resulting download URL and sha256.
func (s *Server) receiveFile(w http.ResponseWriter, r *http.Request) (url, sha256Hex string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return "", "", false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, "missing \"file\" form field: "+err.Error())
		return "", "", false
	}
	defer func() { _ = file.Close() }()

	sha256Hex, size, err := s.storage.Put(r.Context(), file)
	if err != nil {
		httpx.InternalError(w, r, err)
		return "", "", false
	}

	mediaType := header.Header.Get("Content-Type")
	if err := s.repo.UpsertBlob(r.Context(), sha256Hex, size, mediaType); err != nil {
		httpx.InternalError(w, r, err)
		return "", "", false
	}

	return s.cfg.RootURL + "/files/" + sha256Hex, sha256Hex, true
}

func (s *Server) uploadDistributionFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpx.BadRequest(w, "missing distribution id")
		return
	}

	url, sha256Hex, ok := s.receiveFile(w, r)
	if !ok {
		return
	}

	d, err := s.repo.SetDistributionFile(r.Context(), id, url, sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (s *Server) uploadArtifactFormatFile(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		httpx.BadRequest(w, "invalid version")
		return
	}
	formatIndex := 0
	if v := r.URL.Query().Get("formatIndex"); v != "" {
		formatIndex, err = strconv.Atoi(v)
		if err != nil || formatIndex < 0 {
			httpx.BadRequest(w, "invalid formatIndex")
			return
		}
	}

	url, sha256Hex, ok := s.receiveFile(w, r)
	if !ok {
		return
	}

	a, err := s.repo.SetArtifactFormatFile(r.Context(), uuid, version, formatIndex, url, sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"artifact-resolver/internal/errcode"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Ping(r.Context()); err != nil {
		writeErr(w, errcode.CodeNotReady, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
}

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid request body")
		return
	}
	a, err := s.svc.CreateArtifact(r.Context(), req.Name, req.Description)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	as, err := s.svc.ListArtifacts(r.Context(), limit, offset)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, as)
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a, err := s.svc.GetArtifact(r.Context(), name)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid request body")
		return
	}
	v, err := s.svc.CreateVersion(r.Context(), name, req.Version)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vs, err := s.svc.ListVersions(r.Context(), name)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

func (s *Server) handlePublishVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	v, err := s.svc.PublishVersion(r.Context(), name, version)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeprecateVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	v, err := s.svc.DeprecateVersion(r.Context(), name, version)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	if err := s.svc.DeleteVersion(r.Context(), name, version); err != nil {
		writeAPIErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pagination(r *http.Request) (int, int) {
	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
